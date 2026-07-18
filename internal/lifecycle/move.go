package lifecycle

import (
	"fmt"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// CheckMove validates moving mover under newParent (newParent == "" means
// root). Pure: (nodes, ids) in, error out, no I/O. It is the cycle guard for
// the explicit move action — the rule that keeps the org a tree.
//
// The check walks UP from newParent via parent_id (O(depth), no children map):
// if the walk reaches mover, then mover is an ancestor of newParent, so making
// mover a child of newParent would form a cycle — reject. This covers moving a
// node under itself for free (the walk starts at mover immediately). Moving to
// root is always legal; moving under a dead node is rejected (a live node must
// not report to a corpse — the same nearest-live-ancestor invariant Reparent
// keeps).
func CheckMove(nodes []store.Node, moverID, newParentID string) error {
	byID := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = n
	}
	if _, ok := byID[moverID]; !ok {
		return fmt.Errorf("move: node %q not found", moverID)
	}
	if newParentID == "" {
		return nil // to root — always legal
	}
	np, ok := byID[newParentID]
	if !ok {
		return fmt.Errorf("move: new parent %q not found", newParentID)
	}
	if np.State == "dead" {
		return fmt.Errorf("move: cannot report to a dead agent")
	}
	// Walk up from newParent; reaching mover means newParent is inside mover's
	// own subtree, so this move would create a cycle. The seen guard is
	// defensive: no current write path can produce a parent_id cycle that
	// doesn't involve mover, but a future bug or a hand-edited DB could — and a
	// clean rejection beats an infinite walk (a hang is a worse failure than an
	// error by this project's own standard).
	seen := map[string]bool{}
	for cur := newParentID; cur != ""; {
		if cur == moverID {
			return fmt.Errorf("move: cannot move a node under its own descendant")
		}
		if seen[cur] {
			return fmt.Errorf("move: parent chain is corrupt (cycle at %q)", cur)
		}
		seen[cur] = true
		c, ok := byID[cur]
		if !ok {
			break
		}
		cur = c.ParentID
	}
	return nil
}

// MoveTargets returns the node ids that mover can legally move under (excluding
// root, which the caller offers separately) — every node except mover, mover's
// descendants, and dead nodes. Order follows the input node order.
func MoveTargets(nodes []store.Node, moverID string) []store.Node {
	var out []store.Node
	for _, n := range nodes {
		if n.NodeID == moverID {
			continue
		}
		if CheckMove(nodes, moverID, n.NodeID) == nil {
			out = append(out, n)
		}
	}
	return out
}
