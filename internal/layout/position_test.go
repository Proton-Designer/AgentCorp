package layout

import (
	"math/rand"
	"testing"
)

func centerOf(n *Node) int { return n.X + n.W/2 }

// The defining property: a parent sits centered over the span of its children.
func TestPositionParentCenteredOverChildren(t *testing.T) {
	p := &Node{ID: "p", W: 4, H: 3, Children: []*Node{leaf("a", 10), leaf("b", 10)}}
	Position(p, 2, 2)

	a, b := p.Children[0], p.Children[1]
	wantCenter := (centerOf(a) + centerOf(b)) / 2
	if centerOf(p) != wantCenter {
		t.Fatalf("parent center = %d, want %d (midpoint of children)", centerOf(p), wantCenter)
	}
}

// No overlap: each sibling starts at or after the previous one's right edge + gap.
func TestPositionSiblingsDoNotOverlap(t *testing.T) {
	p := &Node{ID: "p", W: 4, H: 3,
		Children: []*Node{leaf("a", 10), leaf("b", 14), leaf("c", 8)}}
	Position(p, 2, 2)

	for i := 1; i < len(p.Children); i++ {
		prev, cur := p.Children[i-1], p.Children[i]
		if cur.X < prev.X+prev.W+2 {
			t.Fatalf("child %d starts at %d, overlaps prev ending at %d (+gap 2)",
				i, cur.X, prev.X+prev.W)
		}
	}
}

// Depth maps to rows.
func TestPositionAssignsRowsByDepth(t *testing.T) {
	p := &Node{ID: "p", W: 4, H: 3, Children: []*Node{leaf("a", 10)}}
	Position(p, 2, 2)
	if p.Y != 0 {
		t.Fatalf("root Y = %d, want 0", p.Y)
	}
	if want := p.H + 2; p.Children[0].Y != want {
		t.Fatalf("child Y = %d, want %d (parent height + vgap)", p.Children[0].Y, want)
	}
}

// Everything lands on-canvas.
func TestPositionNeverNegative(t *testing.T) {
	root := &Node{ID: "r", W: 60, H: 3,
		Children: []*Node{leaf("a", 4), leaf("b", 4)}}
	Position(root, 2, 2)
	var walk func(*Node)
	walk = func(n *Node) {
		if n.X < 0 || n.Y < 0 {
			t.Fatalf("node %s at (%d,%d): negative coordinate", n.ID, n.X, n.Y)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
}

// A wide parent centers its narrow children under itself, not at the left edge.
// This is the case shift() exists for. A naive "just clamp the parent's X"
// fix to the asymmetric bug below BREAKS this test — verified, diff goes to 15.
func TestPositionNarrowChildrenCenteredUnderWideParent(t *testing.T) {
	p := &Node{ID: "p", W: 40, H: 3, Children: []*Node{leaf("a", 4), leaf("b", 4)}}
	Position(p, 2, 2)
	childSpan := (p.Children[1].X + p.Children[1].W) - p.Children[0].X
	childCenter := p.Children[0].X + childSpan/2
	if diff := centerOf(p) - childCenter; diff > 1 || diff < -1 {
		t.Fatalf("children center %d vs parent center %d: off by %d",
			childCenter, centerOf(p), diff)
	}
}

// REGRESSION — asymmetric child widths.
//
// Every test above uses equal-width children, which makes them structurally
// incapable of catching this: averaging child CENTERS and averaging child
// OUTER EDGES are identical iff first.W == last.W. With a narrow-then-wide
// pair they diverge, the parent's X goes negative, shift() fires, and the
// whole subtree is pushed past its own reserved band into its sibling.
//
// Found by executing the algorithm, not reading it. Keep this test forever.
func TestPositionAsymmetricChildrenStayInBand(t *testing.T) {
	n := &Node{ID: "n", W: 60, H: 3, Children: []*Node{leaf("c1", 2), leaf("c2", 100)}}
	Position(n, 2, 2)

	band := n.SubtreeW() // 2 + 2 + 100 = 104
	var right int
	var walk func(*Node)
	walk = func(x *Node) {
		if r := x.X + x.W; r > right {
			right = r
		}
		for _, c := range x.Children {
			walk(c)
		}
	}
	walk(n)

	if right > n.X+band {
		t.Fatalf("subtree extends to %d but its band ends at %d (+%d overflow): "+
			"a right sibling would be overlapped",
			right, n.X+band, right-(n.X+band))
	}
}

// Randomized guard. The named test above pins one known case; this catches the
// shape of bug that case represents. 3.0% of random trees overflowed with the
// centers-averaging version.
func TestPositionRandomTreesStayInBand(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var build func(depth int) *Node
	build = func(depth int) *Node {
		w := 3 + rng.Intn(58)
		n := &Node{ID: "n", W: w, H: 3}
		if depth >= 2 || rng.Float64() < 0.3 {
			return n
		}
		for i := 0; i < 1+rng.Intn(4); i++ {
			n.Children = append(n.Children, build(depth+1))
		}
		return n
	}

	for i := 0; i < 500; i++ {
		root := build(0)
		Position(root, 2, 2)

		var check func(*Node) bool
		check = func(x *Node) bool {
			right := x.X + x.W
			var walk func(*Node)
			walk = func(y *Node) {
				if r := y.X + y.W; r > right {
					right = r
				}
				for _, c := range y.Children {
					walk(c)
				}
			}
			walk(x)
			if right > x.X+x.SubtreeW() {
				return false
			}
			for _, c := range x.Children {
				if !check(c) {
					return false
				}
			}
			return true
		}
		if !check(root) {
			t.Fatalf("tree %d: a subtree overflowed its reserved band", i)
		}
	}
}

// REGRESSION — two levels of asymmetric nesting.
//
// The card-edge fix still failed here (62/10,000 on a randomized battery):
// it references the immediate child's own X/W, but when that child is itself
// narrower than its children's span, its card sits inside its sub-band and
// its edge understates the true extent one level up. Only the memoized
// recursive extent is correct at every depth. 0/10,000 with it.
func TestPositionNestedAsymmetryStaysInBand(t *testing.T) {
	// inner: narrow card (4) over a wide asymmetric pair -> card sits inside
	// its own sub-band, which is exactly what breaks card-edge referencing.
	inner := &Node{ID: "inner", W: 4, H: 3,
		Children: []*Node{leaf("i1", 2), leaf("i2", 80)}}
	root := &Node{ID: "root", W: 50, H: 3,
		Children: []*Node{inner, leaf("r2", 6)}}
	Position(root, 2, 2)

	for _, n := range []*Node{root, inner} {
		lo, hi := n.Extent()
		if hi-lo > n.SubtreeW() {
			t.Fatalf("%s: rendered extent %d wide, band only %d",
				n.ID, hi-lo, n.SubtreeW())
		}
	}
	// The concrete symptom: siblings must not collide.
	iLo, iHi := inner.Extent()
	r2Lo, _ := root.Children[1].Extent()
	if r2Lo < iHi+2 {
		t.Fatalf("sibling starts at %d but inner's subtree ends at %d (gap 2 required); "+
			"inner spans [%d,%d)", r2Lo, iHi, iLo, iHi)
	}
}

func BenchmarkPosition40Nodes(b *testing.B) {
	build := func() *Node {
		root := &Node{ID: "r", W: 16, H: 3}
		for i := 0; i < 5; i++ {
			mid := &Node{ID: "m", W: 16, H: 3}
			for j := 0; j < 7; j++ {
				mid.Children = append(mid.Children, leaf("l", 14))
			}
			root.Children = append(root.Children, mid)
		}
		return root
	}
	for i := 0; i < b.N; i++ {
		Position(build(), 2, 2)
	}
}
