package layout

import "testing"

func hasSeg(segs []Seg, x, y int) bool {
	for _, s := range segs {
		if s.X == x && s.Y == y {
			return true
		}
	}
	return false
}

// A single child gets a straight drop from the parent's center — no bridge.
func TestConnectorsSingleChildIsVertical(t *testing.T) {
	p := &Node{ID: "p", W: 8, H: 3, Children: []*Node{leaf("a", 8)}}
	Position(p, 2, 2)
	segs := Connectors(p, 2)
	if len(segs) == 0 {
		t.Fatal("no connector segments emitted")
	}
	cx := p.X + p.W/2
	for _, s := range segs {
		if s.X != cx {
			t.Fatalf("segment at x=%d, want all at parent center x=%d", s.X, cx)
		}
	}
}

// Two children get a horizontal bridge spanning both child centers.
func TestConnectorsBridgeSpansChildCenters(t *testing.T) {
	p := &Node{ID: "p", W: 8, H: 3, Children: []*Node{leaf("a", 8), leaf("b", 8)}}
	Position(p, 2, 2)
	segs := Connectors(p, 2)

	busY := p.Y + p.H + 1 // the bridge row
	lc := p.Children[0].X + p.Children[0].W/2
	rc := p.Children[1].X + p.Children[1].W/2

	if !hasSeg(segs, lc, busY) {
		t.Fatalf("no segment above left child center (x=%d, y=%d)", lc, busY)
	}
	if !hasSeg(segs, rc, busY) {
		t.Fatalf("no segment above right child center (x=%d, y=%d)", rc, busY)
	}
}

// A collapsed parent emits nothing — no dangling lines to hidden children.
func TestConnectorsCollapsedEmitsNothing(t *testing.T) {
	p := &Node{ID: "p", W: 8, H: 3, Collapsed: true,
		Children: []*Node{leaf("a", 8), leaf("b", 8)}}
	Position(p, 2, 2)
	if segs := Connectors(p, 2); len(segs) != 0 {
		t.Fatalf("collapsed node emitted %d segments, want 0", len(segs))
	}
}

func TestConnectorsLeafEmitsNothing(t *testing.T) {
	n := leaf("a", 8)
	Position(n, 2, 2)
	if segs := Connectors(n, 2); len(segs) != 0 {
		t.Fatalf("leaf emitted %d segments, want 0", len(segs))
	}
}
