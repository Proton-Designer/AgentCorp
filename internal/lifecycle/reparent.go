// Package lifecycle implements the pure tree-surgery decisions around firing
// a node: who reparents to whom, who gets disbanded and in what order, and
// who needs to be told. Everything here is a pure function — nodes and a
// target in, a decision out. No process is killed, no store is written; the
// caller performs every side effect.
package lifecycle

import (
	"fmt"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Move is a proposed parent_id change for one node.
type Move struct {
	NodeID      string
	OldParentID string
	NewParentID string
}

// Reparent decides where victim's children go when victim is fired
// (non-cascading fire, per spec §6.3's default): each direct child reattaches
// to the nearest LIVE ancestor above victim, never to victim's immediate
// parent blindly — a fired node's own parent might ALSO already be dead
// (tombstoned, row retained per the tombstone-not-prune policy), and
// attaching a child under a dead node would leave it with no functioning
// manager at all. If every ancestor up to the top is dead, children become
// new roots.
//
// nodes must contain victim's full ancestor chain for a correct result — a
// parent_id that isn't present in the given slice is treated as reaching a
// root (a defensive fallback for a caller-contract violation, not something
// this function can fully validate on its own).
func Reparent(nodes []store.Node, victim string) ([]Move, error) {
	byID := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = n
	}

	v, ok := byID[victim]
	if !ok {
		return nil, fmt.Errorf("reparent: node %q not found", victim)
	}

	newParent, err := nearestLiveAncestor(byID, v.ParentID)
	if err != nil {
		return nil, fmt.Errorf("reparent: %w", err)
	}

	var moves []Move
	for _, n := range nodes {
		if n.ParentID == victim {
			moves = append(moves, Move{NodeID: n.NodeID, OldParentID: victim, NewParentID: newParent})
		}
	}
	return moves, nil
}

// nearestLiveAncestor walks up the parent chain starting at startParentID,
// skipping dead (tombstoned) ancestors, until it finds a live one, reaches a
// root (empty parent_id), or falls off the edge of the given node set
// (treated as reaching a root). Guards against a cycle in parent_id — which
// should never occur given FK enforcement plus sane reparent logic, but a
// pure function operating on an arbitrary slice must not trust that and must
// not hang: a naive unguarded walk would loop forever on a cyclic chain.
func nearestLiveAncestor(byID map[string]store.Node, startParentID string) (string, error) {
	visited := map[string]bool{}
	cur := startParentID
	for cur != "" {
		if visited[cur] {
			return "", fmt.Errorf("cycle detected in parent chain at node %q", cur)
		}
		visited[cur] = true

		n, ok := byID[cur]
		if !ok {
			return "", nil // dangling reference -- treat as reaching a root
		}
		if n.State != "dead" {
			return cur, nil
		}
		cur = n.ParentID
	}
	return "", nil
}
