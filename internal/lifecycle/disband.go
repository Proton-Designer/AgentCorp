package lifecycle

import (
	"fmt"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Disband decides the full cascade for the explicit "disband team" action
// (spec §6.3) — never the default fire, which uses Reparent instead. Returns
// every node in root's subtree, root included, in kill order: post-order,
// so a node is never returned before any of its own descendants. That's the
// actual invariant that matters — killing a parent before its children would
// strand them mid-tick, still "alive" and pointing at a manager that's
// already gone. Ordering between unrelated branches is unconstrained; it has
// no bearing on correctness since neither is an ancestor of the other.
//
// Pending nodes are included like any other — Disband only decides what's
// affected, not how each affected node is actually terminated (a pending
// node has no live process to signal; that's the caller's concern).
func Disband(nodes []store.Node, root string) ([]store.Node, error) {
	byID := make(map[string]store.Node, len(nodes))
	childrenOf := make(map[string][]store.Node, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = n
		if n.ParentID != "" {
			childrenOf[n.ParentID] = append(childrenOf[n.ParentID], n)
		}
	}

	if _, ok := byID[root]; !ok {
		return nil, fmt.Errorf("disband: node %q not found", root)
	}

	var order []store.Node
	visited := map[string]bool{}
	var visit func(id string) error
	visit = func(id string) error {
		// A cycle in parent_id would make a node transitively its own
		// descendant, which would recurse forever under a naive walk.
		// Detected here instead: revisiting a node already on this walk
		// means the subtree loops back on itself.
		if visited[id] {
			return fmt.Errorf("disband: cycle detected at node %q", id)
		}
		visited[id] = true
		for _, c := range childrenOf[id] {
			if err := visit(c.NodeID); err != nil {
				return err
			}
		}
		order = append(order, byID[id])
		return nil
	}

	if err := visit(root); err != nil {
		return nil, err
	}
	return order, nil
}
