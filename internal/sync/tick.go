package sync

import (
	"fmt"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// TickMsg is one poll cycle's result. It has no dependency on Bubble Tea —
// tea.Msg is `interface{}`, so any concrete type satisfies it, and this
// package has no reason to import a UI framework to produce data for one.
//
// Err is set whenever ANY part of the tick failed (tmux unreachable, broker
// unreachable, store read failure). When Err != nil, Reconciliation/PaneDiff
// reflect only what was gathered before the failure — the UI must treat a
// non-nil Err as "unknown," never infer death or absence from it.
type TickMsg struct {
	PaneDiff       PaneDiff
	Reconciliation broker.Reconciliation
	Applied        Actions
	Err            error
}

// Tick runs one poll-diff-reconcile-apply cycle.
//
// listPanes and listPeers are injected rather than called directly so this
// can be tested without a live tmux server or a real claude-peers broker —
// the orchestration logic is what's under test here, not those two external
// systems (which have their own dedicated tests: the real-tmux integration
// test in tmux_test.go, and broker's own suite).
//
// The hard invariant, stated plainly because it's the expensive one to get
// wrong: any failed poll must change NOTHING in the store. A transient tmux
// or broker hiccup must never cascade into tombstoning live nodes — that's
// the single most expensive failure mode this package can produce, because
// tombstone is the one state this design treats as terminal. Both I/O calls
// are checked before any store write is attempted.
func Tick(
	listPanes func() (map[string]Pane, error),
	listPeers func() ([]broker.Peer, error),
	st *store.Store,
	lastPanes map[string]Pane,
) (TickMsg, map[string]Pane) {
	curPanes, err := listPanes()
	if err != nil {
		// Poll failed -- unknown, not empty. Don't advance lastPanes (we have
		// no new snapshot to advance to) and don't touch the store.
		return TickMsg{Err: fmt.Errorf("tick: %w", err)}, lastPanes
	}
	diff := DiffPanes(lastPanes, curPanes)

	peers, err := listPeers()
	if err != nil {
		// tmux side succeeded, so we DO advance lastPanes (that signal was
		// real and shouldn't be thrown away just because the broker call
		// failed independently) -- but we take no store action at all.
		return TickMsg{PaneDiff: diff, Err: fmt.Errorf("tick: %w", err)}, curPanes
	}

	nodes, err := st.ListNodes()
	if err != nil {
		return TickMsg{PaneDiff: diff, Err: fmt.Errorf("tick: %w", err)}, curPanes
	}

	recon := broker.Reconcile(nodes, peers)
	actions := Decide(diff, recon, nodes)

	// Outer bound on pending hires: a still-unbound pending node older than the
	// grace never registered — fail it (and reap its orphaned pane in apply)
	// rather than let it sit pending forever. A node about to be bound THIS tick
	// registered (just slowly) and is excluded — binding wins.
	binding := make(map[string]bool, len(actions.Bind))
	for _, b := range actions.Bind {
		binding[b.NodeID] = true
	}
	for _, id := range StalePending(nodes, time.Now().UTC(), PendingGrace) {
		if !binding[id] {
			actions.Fail = append(actions.Fail, id)
		}
	}

	if err := apply(st, actions); err != nil {
		return TickMsg{PaneDiff: diff, Reconciliation: recon, Err: fmt.Errorf("tick: apply: %w", err)}, curPanes
	}

	return TickMsg{PaneDiff: diff, Reconciliation: recon, Applied: actions}, curPanes
}

// apply is the I/O edge that turns Actions into store writes.
//
// Order matters: Tombstone runs fully before Bind so that if a node is proposed
// for both in one tick (a pane died AND a peer appeared on its reused tty),
// BindPeer's WHERE state='pending' guard rejects the now-dead node with a clean
// RowsAffected==0 error rather than resurrecting it. Fail runs last, only for
// nodes Decide didn't already tombstone or bind.
func apply(st *store.Store, a Actions) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, nodeID := range a.Tombstone {
		if err := st.Tombstone(nodeID, now); err != nil {
			return fmt.Errorf("tombstone %s: %w", nodeID, err)
		}
	}
	for _, b := range a.Bind {
		if err := st.BindPeer(b.NodeID, b.PeerID); err != nil {
			return fmt.Errorf("bind %s<-%s: %w", b.NodeID, b.PeerID, err)
		}
	}
	for _, nodeID := range a.Fail {
		// Reap the orphaned pane first (best-effort) so a permanently-broken
		// hire doesn't leave a dangling tmux window with no signal to clean it
		// up, then mark the node failed so the operator sees where it went.
		reapPane(st, nodeID)
		if err := st.SetState(nodeID, "failed"); err != nil {
			return fmt.Errorf("fail %s: %w", nodeID, err)
		}
	}
	return nil
}

// reapPane kills the tmux pane a node was spawned into, if any. Best-effort: a
// pane that's already gone is success, and a lookup failure must not block the
// state change that tells the operator the hire is over.
func reapPane(st *store.Store, nodeID string) {
	nodes, err := st.ListNodes()
	if err != nil {
		return
	}
	for _, n := range nodes {
		if n.NodeID == nodeID && n.SpawnRef != "" {
			_ = broker.KillPane(n.SpawnRef)
			return
		}
	}
}
