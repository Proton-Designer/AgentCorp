package layout

import "testing"

func leaf(id string, w int) *Node { return &Node{ID: id, W: w, H: 3} }

func TestMeasureLeafIsOwnWidth(t *testing.T) {
	n := leaf("a", 10)
	if got := Measure(n, 2); got != 10 {
		t.Fatalf("Measure(leaf w=10) = %d, want 10", got)
	}
}

// A parent narrower than its children must widen to span them, or siblings
// overlap — the core failure mode this algorithm exists to prevent.
func TestMeasureParentSpansChildren(t *testing.T) {
	p := &Node{ID: "p", W: 4, H: 3, Children: []*Node{leaf("a", 10), leaf("b", 10)}}
	// children 10 + 10, gap 2 => 22; parent's own 4 is narrower, so 22 wins.
	if got := Measure(p, 2); got != 22 {
		t.Fatalf("Measure = %d, want 22", got)
	}
}

// A parent wider than its children keeps its own width.
func TestMeasureWideParentKeepsOwnWidth(t *testing.T) {
	p := &Node{ID: "p", W: 40, H: 3, Children: []*Node{leaf("a", 5), leaf("b", 5)}}
	if got := Measure(p, 2); got != 40 {
		t.Fatalf("Measure = %d, want 40 (parent wider than children's span)", got)
	}
}

// Collapse must make a node measure as a leaf — this is what makes §7's fold
// an O(1) scale lever rather than a special case.
func TestMeasureCollapsedNodeIsLeaf(t *testing.T) {
	p := &Node{ID: "p", W: 6, H: 3, Collapsed: true,
		Children: []*Node{leaf("a", 100), leaf("b", 100)}}
	if got := Measure(p, 2); got != 6 {
		t.Fatalf("Measure(collapsed) = %d, want 6", got)
	}
}

func TestMeasureThreeLevels(t *testing.T) {
	// root over two parents, each over two 8-wide leaves.
	mk := func(id string) *Node {
		return &Node{ID: id, W: 4, H: 3, Children: []*Node{leaf(id+"1", 8), leaf(id+"2", 8)}}
	}
	root := &Node{ID: "r", W: 4, H: 3, Children: []*Node{mk("a"), mk("b")}}
	// each parent spans 8+2+8 = 18; root spans 18+2+18 = 38
	if got := Measure(root, 2); got != 38 {
		t.Fatalf("Measure(3-level) = %d, want 38", got)
	}
}
