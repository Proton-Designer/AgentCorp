// Package ui paints the positioned layout tree and hosts the Bubble Tea
// program. It is the only package that knows about terminals.
package ui

import (
	"strings"

	"github.com/aymanmohammed/crew/internal/layout"
)

// Render lays out the tree and paints it into a rune grid.
//
// A rune grid rather than Lipgloss's Canvas because there is nothing to style
// yet — NOT because Canvas is hard to test (it exposes a plain .Render() with
// no live-terminal dependency, so it would be equally CI-testable). Recording
// the real reason so this isn't later cited as a case against Canvas.
//
// DEFERRED COST, tracked (REQUIREMENTS UI-7): the width checks in tests use
// utf8.RuneCountInString, which assumes 1 rune = 1 terminal cell. True for
// Phase 1's ASCII fixtures, FALSE for the role/status glyphs the spec
// specifies (◆ ⚙ ✦ ● ○ ◍) — several are East-Asian-Ambiguous and can render
// double-width, which would silently corrupt the 80/120-column exit criteria
// with no test failure. Phase 2's styling pass MUST route width through
// clipperhouse/displaywidth (already in Lipgloss's go.mod).
func Render(root *layout.Node, width int) string {
	if root == nil {
		return ""
	}
	layout.Position(root, hgap, vgap)
	segs := layout.Connectors(root, vgap)

	w, h := extent(root)
	if w < 1 || h < 1 {
		return ""
	}

	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", w))
	}
	put := func(x, y int, r rune) {
		if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[y]) {
			grid[y][x] = r
		}
	}

	// Connectors first; cards paint over them. Segments are applied in the
	// order Connectors emitted them — corners and tees deliberately overwrite
	// the bus at shared cells.
	for _, s := range segs {
		put(s.X, s.Y, s.Rune)
	}

	var cards func(*layout.Node)
	cards = func(n *layout.Node) {
		drawCard(put, n)
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			cards(c)
		}
	}
	cards(root)

	// Center the diagram in the viewport. "Centered org chart" is as much
	// about the canvas as about the parent/child relationship.
	pad := 0
	if width > w {
		pad = (width - w) / 2
	}
	prefix := strings.Repeat(" ", pad)

	var b strings.Builder
	for i, row := range grid {
		if i > 0 {
			b.WriteByte('\n')
		}
		line := strings.TrimRight(string(row), " ")
		if line != "" {
			b.WriteString(prefix)
			b.WriteString(line)
		}
	}
	return b.String()
}

const (
	hgap = 2
	vgap = 2
)

// drawCard renders a rounded box with the node's label centered inside.
func drawCard(put func(int, int, rune), n *layout.Node) {
	x, y, w, h := n.X, n.Y, n.W, n.H
	if w < 2 || h < 2 {
		return
	}
	for i := 1; i < w-1; i++ {
		put(x+i, y, '─')
		put(x+i, y+h-1, '─')
	}
	for j := 1; j < h-1; j++ {
		put(x, y+j, '│')
		put(x+w-1, y+j, '│')
	}
	put(x, y, '╭')
	put(x+w-1, y, '╮')
	put(x, y+h-1, '╰')
	put(x+w-1, y+h-1, '╯')

	label := []rune(n.ID)
	if len(label) > w-2 {
		label = label[:w-2]
	}
	start := x + (w-len(label))/2
	for i, r := range label {
		put(start+i, y+h/2, r)
	}
}

// extent returns the bounding box of the positioned tree, honouring collapse.
func extent(root *layout.Node) (w, h int) {
	var walk func(*layout.Node)
	walk = func(n *layout.Node) {
		if r := n.X + n.W; r > w {
			w = r
		}
		if b := n.Y + n.H; b > h {
			h = b
		}
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return w, h
}
