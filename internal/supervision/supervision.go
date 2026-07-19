// Package supervision turns AgentCorp's already-shipped memory-intact revival
// (hire.Flow.Revive, backed by `claude --resume`) into automatic, policy-driven,
// Erlang/OTP-style fault tolerance: restart strategies, budget-bounded restarts,
// and escalation.
//
// Evaluate is the only entry point and is pure — nodes, newly-dead node IDs,
// restart history, and policies in; a Plan out. No I/O, no store write, same
// discipline as sync.Decide and lifecycle.Reparent/Disband: this package only
// decides; the caller (hire.Flow.Revive, and whatever kills a swept-in living
// sibling) performs every side effect.
//
// The data-model types (Strategy, Policy, RestartEvent) live in package store,
// not here — store.Node already flows one-way into this package (matching
// lifecycle.Reparent/broker.Reconcile's existing directionality), and store
// also needs those types for its own CRUD (InsertRestartEvent,
// UpsertSupervisionPolicy, ...). Defining them here instead would make store
// import supervision AND supervision import store — an import cycle. Keeping
// every store-shaped type on the store side, and only decision LOGIC here,
// is the same boundary this codebase already draws around store.Node/store.Role.
//
// Deliberate divergence from literal OTP semantics, stated here because it is
// the one place this design does NOT copy Erlang verbatim: real OTP terminates
// a supervisor that exceeds its restart budget, which can cascade into killing
// and restarting an otherwise-healthy supervisor process. This package never
// does that — a Claude Code agent is expensive and its state is a real,
// valuable conversation, not a cheap OS process. Exceeding a budget here
// produces ActionEscalate (a signal to the parent) or ActionAlertRoot (a
// human-visible alert at the top), never an autonomous kill of a currently
// healthy node. See supervision-design.md for the full reasoning.
package supervision

import (
	"sort"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// DefaultPolicy applies to any supervisor with no explicit store.Policy row:
// restart only the dead child, small budget. The safest, least-surprising
// default, and the one that matches what a single 'z' press already does today.
var DefaultPolicy = store.Policy{Strategy: store.OneForOne, MaxRestarts: 3, WindowSeconds: 300}

// Action is what should happen to one node as a result of processing a death.
type Action string

const (
	// ActionRevive: the node is dead; attempt hire.Flow.Revive on it.
	ActionRevive Action = "revive"

	// ActionKillAndRestart: the node is currently ALIVE but was swept into a
	// one-for-all/rest-for-one sibling group by another node's death. This is
	// a strictly bigger, more destructive operation than reviving an
	// already-dead node (it requires killing a working process first) and
	// must never be conflated with ActionRevive by a caller that only checks
	// "is this a restart decision."
	ActionKillAndRestart Action = "kill_and_restart"

	// ActionEscalate: the dead node's supervisor is at or over its restart
	// budget. The node is NOT restarted. The caller should surface this to
	// SupervisorID (a message, a flagged UI state) rather than retry.
	ActionEscalate Action = "escalate"

	// ActionAlertRoot: escalation reached a node with no parent to escalate
	// to (SupervisorID has no ParentID, or the dead node itself is the root).
	// Human-visible alert; never an autonomous action.
	ActionAlertRoot Action = "alert_root"
)

// Decision is one node's outcome.
type Decision struct {
	NodeID       string
	Action       Action
	SupervisorID string // the node whose policy/budget produced this decision
	Reason       string
}

// Plan is the full result of evaluating one tick's newly-dead nodes.
type Plan struct {
	Decisions []Decision
}

// Evaluate is the pure supervision-decision step. See package doc for the
// full contract; see supervision-design.md for the algorithm's step-by-step
// rationale (this implementation follows it exactly).
func Evaluate(nodes []store.Node, dead []string, history []store.RestartEvent, policies []store.Policy, now time.Time) Plan {
	byID := make(map[string]store.Node, len(nodes))
	childrenOf := make(map[string][]store.Node, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = n
		if n.ParentID != "" {
			childrenOf[n.ParentID] = append(childrenOf[n.ParentID], n)
		}
	}

	policyByID := make(map[string]store.Policy, len(policies))
	for _, p := range policies {
		policyByID[p.NodeID] = p
	}
	policyFor := func(nodeID string) store.Policy {
		if p, ok := policyByID[nodeID]; ok {
			return p
		}
		return DefaultPolicy
	}

	// Restart count per supervisor, scoped to nodes that are CURRENTLY its
	// children (a documented simplification — see supervision-design.md's
	// algorithm step 3 for why historical topology isn't reconstructed).
	restartCount := func(supervisorID string, windowSeconds int) int {
		kids := make(map[string]bool, len(childrenOf[supervisorID]))
		for _, c := range childrenOf[supervisorID] {
			kids[c.NodeID] = true
		}
		window := time.Duration(windowSeconds) * time.Second
		n := 0
		for _, ev := range history {
			if !kids[ev.NodeID] {
				continue
			}
			if now.Sub(ev.At) <= window { // inclusive at the boundary, pinned deliberately
				n++
			}
		}
		return n
	}

	// Stable, deterministic order: two calls on the same snapshot must
	// produce a byte-identical Plan.
	deadSorted := append([]string(nil), dead...)
	sort.Strings(deadSorted)

	decided := map[string]bool{}
	var decisions []Decision

	for _, id := range deadSorted {
		if decided[id] {
			continue
		}
		d, ok := byID[id]
		if !ok {
			continue // not a known node; nothing to decide
		}
		if d.ParentID == "" {
			decisions = append(decisions, Decision{
				NodeID: id, Action: ActionAlertRoot, SupervisorID: "",
				Reason: "root has no supervisor",
			})
			decided[id] = true
			continue
		}

		p, ok := byID[d.ParentID]
		if !ok {
			// Dangling parent reference; treat like a rootless node rather
			// than crash on a lookup that can't succeed.
			decisions = append(decisions, Decision{
				NodeID: id, Action: ActionAlertRoot, SupervisorID: d.ParentID,
				Reason: "supervisor not found",
			})
			decided[id] = true
			continue
		}
		policy := policyFor(p.NodeID)

		if restartCount(p.NodeID, policy.WindowSeconds) < policy.MaxRestarts {
			group := siblingGroup(d, childrenOf[p.NodeID], policy.Strategy)
			for _, n := range group {
				if decided[n.NodeID] {
					continue
				}
				action := ActionRevive
				reason := "restarting dead node"
				if n.State != "dead" {
					action = ActionKillAndRestart
					reason = "swept into " + string(policy.Strategy) + " restart group"
				}
				decisions = append(decisions, Decision{
					NodeID: n.NodeID, Action: action, SupervisorID: p.NodeID, Reason: reason,
				})
				decided[n.NodeID] = true
			}
			continue
		}

		decisions = append(decisions, Decision{
			NodeID: id, Action: ActionEscalate, SupervisorID: p.NodeID,
			Reason: "supervisor restart budget exceeded",
		})
		decided[id] = true
		if p.ParentID == "" {
			decisions = append(decisions, Decision{
				NodeID: id, Action: ActionAlertRoot, SupervisorID: p.NodeID,
				Reason: "escalation reached root",
			})
		}
	}

	return Plan{Decisions: decisions}
}

// siblingGroup computes which of P's children a restart strategy sweeps in,
// given that d (a child of P) just died.
func siblingGroup(d store.Node, siblings []store.Node, strategy store.Strategy) []store.Node {
	switch strategy {
	case store.OneForAll:
		return siblings
	case store.RestForOne:
		var group []store.Node
		for _, s := range siblings {
			if s.CreatedAt >= d.CreatedAt { // RFC3339 lexicographic == chronological
				group = append(group, s)
			}
		}
		return group
	default: // OneForOne and any unrecognized value fall back to the safest case
		return []store.Node{d}
	}
}
