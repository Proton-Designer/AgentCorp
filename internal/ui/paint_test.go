package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
)

// threeLevelTree builds the 7-node shape from the spec's hero mockup:
// one root, two leads, four ICs.
func threeLevelTree() *layout.Node {
	mk := func(id string) *layout.Node {
		return &layout.Node{ID: id, W: 12, H: 3, Children: []*layout.Node{
			{ID: id + "1", W: 12, H: 3},
			{ID: id + "2", W: 12, H: 3},
		}}
	}
	return &layout.Node{ID: "ceo", W: 12, H: 3,
		Children: []*layout.Node{mk("be"), mk("fe")}}
}

// EXIT CRITERION 1+2: renders inside an 80-column terminal.
func TestRenderFitsAt80Columns(t *testing.T) {
	out := Render(threeLevelTree(), 80)
	for i, line := range strings.Split(out, "\n") {
		if w := utf8.RuneCountInString(line); w > 80 {
			t.Fatalf("line %d is %d cells wide, want <= 80:\n%s", i, w, line)
		}
	}
}

func TestRenderFitsAt120Columns(t *testing.T) {
	out := Render(threeLevelTree(), 120)
	for i, line := range strings.Split(out, "\n") {
		if w := utf8.RuneCountInString(line); w > 120 {
			t.Fatalf("line %d is %d cells wide, want <= 120", i, w)
		}
	}
}

// The whole point of Decision #4: the root is centered over its children.
func TestRenderRootIsCenteredOverChildren(t *testing.T) {
	root := threeLevelTree()
	layout.Position(root, 2, 2)
	rootCenter := root.X + root.W/2
	l, r := root.Children[0], root.Children[1]
	childrenCenter := ((l.X + l.W/2) + (r.X + r.W/2)) / 2
	if diff := rootCenter - childrenCenter; diff > 1 || diff < -1 {
		t.Fatalf("root center %d, children center %d — not centered", rootCenter, childrenCenter)
	}
}

func TestRenderIncludesEveryNodeID(t *testing.T) {
	out := Render(threeLevelTree(), 120)
	for _, id := range []string{"ceo", "be", "fe", "be1", "be2", "fe1", "fe2"} {
		if !strings.Contains(out, id) {
			t.Fatalf("rendered output missing node %q:\n%s", id, out)
		}
	}
}

// No node may be painted over another. Card interiors must not collide.
func TestRenderNoOverlappingCards(t *testing.T) {
	root := threeLevelTree()
	layout.Position(root, 2, 2)

	type box struct{ x, y, w, h int }
	var boxes []box
	var walk func(*layout.Node)
	walk = func(n *layout.Node) {
		boxes = append(boxes, box{n.X, n.Y, n.W, n.H})
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	for i := 0; i < len(boxes); i++ {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			xo := a.x < b.x+b.w && b.x < a.x+a.w
			yo := a.y < b.y+b.h && b.y < a.y+a.h
			if xo && yo {
				t.Fatalf("cards overlap: (%d,%d,%d,%d) and (%d,%d,%d,%d)",
					a.x, a.y, a.w, a.h, b.x, b.y, b.w, b.h)
			}
		}
	}
}

func TestRenderNilRootIsEmpty(t *testing.T) {
	if got := Render(nil, 80); got != "" {
		t.Fatalf("Render(nil) = %q, want empty", got)
	}
}

// EXIT CRITERION 3: collapse hides the subtree and re-layouts smaller.
func TestCollapseHidesSubtreeAndShrinks(t *testing.T) {
	root := threeLevelTree()
	before := Render(root, 120)

	root.Children[0].Collapsed = true
	after := Render(root, 120)

	if strings.Contains(after, "be1") {
		t.Fatal("collapsed subtree still renders child be1")
	}
	if !strings.Contains(after, "fe1") {
		t.Fatal("collapsing one branch wrongly hid a sibling branch")
	}
	if len(after) >= len(before) {
		t.Fatal("collapsed render is not smaller — layout did not recompute")
	}
}

func TestExpandRestores(t *testing.T) {
	root := threeLevelTree()
	before := Render(root, 120)
	root.Children[0].Collapsed = true
	Render(root, 120)
	root.Children[0].Collapsed = false
	if after := Render(root, 120); after != before {
		t.Fatal("expand did not restore the original render")
	}
}
