// Package ui paints the positioned layout tree and hosts the Bubble Tea
// program. It is the only package that knows about terminals.
package ui

import (
	"strings"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// buildGrid positions the tree and paints it into a rune grid plus a parallel
// style grid. styleOf resolves a node's card colour by its ID; connectors are
// always styConnector; empty cells stay styNone. Both the plain and the styled
// renderers below share this one builder, so colour can never drift from
// geometry — the exact same cells are drawn either way.
func buildGrid(root *layout.Node, styleOf func(id string) cellStyle) (grid [][]rune, styles [][]cellStyle, w, h int) {
	layout.Position(root, hgap, vgap)
	segs := layout.Connectors(root, vgap)

	w, h = extent(root)
	if w < 1 || h < 1 {
		return nil, nil, 0, 0
	}

	grid = make([][]rune, h)
	styles = make([][]cellStyle, h)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", w))
		styles[i] = make([]cellStyle, w)
	}
	put := func(x, y int, r rune, s cellStyle) {
		if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[y]) {
			grid[y][x] = r
			styles[y][x] = s
		}
	}

	// Connectors first; cards paint over them. Segments are applied in the
	// order Connectors emitted them — corners and tees deliberately overwrite
	// the bus at shared cells.
	for _, s := range segs {
		put(s.X, s.Y, s.Rune, styConnector)
	}

	var cards func(*layout.Node)
	cards = func(n *layout.Node) {
		drawCard(put, n, styleOf(n.ID))
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			cards(c)
		}
	}
	cards(root)
	return grid, styles, w, h
}

// Render lays out the tree and paints it into a monochrome string. This is the
// geometry-only path: the layout tests assert against it, and it never emits
// ANSI, so a width count of its output is a true cell count.
//
// DEFERRED COST, tracked (REQUIREMENTS UI-7): the width checks in tests use
// utf8.RuneCountInString, which assumes 1 rune = 1 terminal cell. True for the
// ASCII fixtures, FALSE for East-Asian-Ambiguous glyphs. Tracked, not yet done.
func Render(root *layout.Node, width int) string {
	if root == nil {
		return ""
	}
	grid, _, w, h := buildGrid(root, func(string) cellStyle { return styNone })
	if w < 1 || h < 1 {
		return ""
	}
	prefix := centerPad(width, w)

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

// RenderStyled is Render plus colour: each card is painted in its live status
// colour and the connectors are dimmed, by emitting one ANSI run per contiguous
// same-style span. Geometry is identical to Render — ANSI escapes are
// zero-width, so centring and alignment are unaffected. Falls back to the plain
// renderer when colour is disabled (NO_COLOR).
func RenderStyled(root *layout.Node, width int, statusOf func(id string) vitals.Status) string {
	if root == nil {
		return ""
	}
	if !colorEnabled {
		return Render(root, width)
	}
	grid, styles, w, h := buildGrid(root, func(id string) cellStyle {
		return styleForStatus(statusOf(id))
	})
	if w < 1 || h < 1 {
		return ""
	}
	prefix := centerPad(width, w)

	var b strings.Builder
	for i := range grid {
		if i > 0 {
			b.WriteByte('\n')
		}
		emitStyledRow(&b, grid[i], styles[i], prefix)
	}
	return b.String()
}

// emitStyledRow writes one row with ANSI colour, trailing blanks trimmed. A
// blank line writes nothing (matching Render). Contiguous cells of the same
// style share one escape; the run is closed with a reset before the next.
func emitStyledRow(b *strings.Builder, runes []rune, styles []cellStyle, prefix string) {
	end := len(runes)
	for end > 0 && runes[end-1] == ' ' {
		end--
	}
	if end == 0 {
		return
	}
	b.WriteString(prefix)
	cur := styNone
	open := false
	for i := 0; i < end; i++ {
		if styles[i] != cur {
			if open {
				b.WriteString(ansiReset)
				open = false
			}
			if code := ansiFor(styles[i]); code != "" {
				b.WriteString(code)
				open = true
			}
			cur = styles[i]
		}
		b.WriteRune(runes[i])
	}
	if open {
		b.WriteString(ansiReset)
	}
}

// centerPad returns the leading spaces that centre a w-wide diagram in width.
func centerPad(width, w int) string {
	if width > w {
		return strings.Repeat(" ", (width-w)/2)
	}
	return ""
}

const (
	hgap = 2
	vgap = 2
)

// drawCard renders a rounded box with the node's label centered inside, every
// cell painted in style s (styNone for the monochrome path).
func drawCard(put func(int, int, rune, cellStyle), n *layout.Node, s cellStyle) {
	x, y, w, h := n.X, n.Y, n.W, n.H
	if w < 2 || h < 2 {
		return
	}
	for i := 1; i < w-1; i++ {
		put(x+i, y, '─', s)
		put(x+i, y+h-1, '─', s)
	}
	for j := 1; j < h-1; j++ {
		put(x, y+j, '│', s)
		put(x+w-1, y+j, '│', s)
	}
	put(x, y, '╭', s)
	put(x+w-1, y, '╮', s)
	put(x, y+h-1, '╰', s)
	put(x+w-1, y+h-1, '╯', s)

	label := []rune(n.ID)
	if len(label) > w-2 {
		label = label[:w-2]
	}
	start := x + (w-len(label))/2
	for i, r := range label {
		put(start+i, y+h/2, r, s)
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
