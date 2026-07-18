package lifecycle

import (
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func moveTree() []store.Node {
	return []store.Node{
		{NodeID: "root", State: "alive"},
		{NodeID: "a", ParentID: "root", State: "alive"},
		{NodeID: "b", ParentID: "root", State: "alive"},
		{NodeID: "a1", ParentID: "a", State: "alive"},
		{NodeID: "a2", ParentID: "a1", State: "alive"},
		{NodeID: "dead1", ParentID: "root", State: "dead"},
	}
}

func TestCheckMoveLegalCases(t *testing.T) {
	n := moveTree()
	// a under b — fine (b is not in a's subtree).
	if err := CheckMove(n, "a", "b"); err != nil {
		t.Fatalf("a under b should be legal: %v", err)
	}
	// a to root — always legal.
	if err := CheckMove(n, "a", ""); err != nil {
		t.Fatalf("move to root should be legal: %v", err)
	}
}

func TestCheckMoveRejectsCycles(t *testing.T) {
	n := moveTree()
	// a under itself.
	if CheckMove(n, "a", "a") == nil {
		t.Fatal("moving a under itself must be rejected")
	}
	// a under a2 — a2 is a's own descendant (a -> a1 -> a2).
	if CheckMove(n, "a", "a2") == nil {
		t.Fatal("moving a under its descendant a2 must be rejected")
	}
	// a under a1 — direct descendant.
	if CheckMove(n, "a", "a1") == nil {
		t.Fatal("moving a under its child a1 must be rejected")
	}
}

func TestCheckMoveRejectsDeadParent(t *testing.T) {
	n := moveTree()
	if CheckMove(n, "b", "dead1") == nil {
		t.Fatal("moving under a dead node must be rejected")
	}
}

func TestMoveTargetsExcludesSelfDescendantsAndDead(t *testing.T) {
	n := moveTree()
	targets := map[string]bool{}
	for _, tt := range MoveTargets(n, "a") {
		targets[tt.NodeID] = true
	}
	// Legal: root, b. Illegal: a (self), a1/a2 (descendants), dead1 (dead).
	if !targets["root"] || !targets["b"] {
		t.Fatalf("expected root and b as targets, got %v", targets)
	}
	for _, bad := range []string{"a", "a1", "a2", "dead1"} {
		if targets[bad] {
			t.Fatalf("%q must not be a legal move target", bad)
		}
	}
}

// The root moved under another node is rejected by the general cycle check with
// no special case — root is every other node's ancestor, so the walk up from
// any target reaches root immediately.
func TestCheckMoveRejectsRootUnderOther(t *testing.T) {
	n := moveTree()
	if CheckMove(n, "root", "b") == nil {
		t.Fatal("moving the root under another node must be rejected")
	}
}

// A corrupt parent_id cycle (not involving the mover) must be rejected cleanly,
// never walked forever.
func TestCheckMoveRejectsCorruptCycle(t *testing.T) {
	n := []store.Node{
		{NodeID: "x", ParentID: "y", State: "alive"},
		{NodeID: "y", ParentID: "x", State: "alive"}, // x<->y cycle
		{NodeID: "m", State: "alive"},
	}
	if CheckMove(n, "m", "x") == nil {
		t.Fatal("a corrupt parent cycle must be rejected, not walked forever")
	}
}
