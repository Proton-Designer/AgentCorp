package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// Office-view layout constants. Fixed cell budgets rather than width-derived
// ones: a desk that shrinks to fit is illegible, so tiles stay a constant
// size and the floor plan wraps/truncates around them instead (packRooms,
// buildRoom's overflow tile).
const (
	officeDeskNameW = 10                  // name budget inside a desk tile
	officeDeskW     = officeDeskNameW + 4 // "[" + marker + " " + name + "]"
	officeDeskGapX  = 1                   // space between desks in a room row
	officeRoomPadX  = 1                   // space padding inside a room's side walls
	officeRoomGapX  = 2                   // space between rooms on the same floor row
	officeRoomGapY  = 1                   // blank line between floor rows
	officeMaxCols   = 4                   // desk-grid column cap per room (keeps rooms squarish)
	officeMaxRows   = 4                   // desk-grid row cap per room; the rest folds into "+N more"
)

// officeSeg is one run of same-styled text within a rendered office line.
// Lines are built as ordered segments rather than a full rune/style grid
// (unlike paint.go's buildGrid) because the office view never overlaps
// elements — rooms sit side by side on a floor row, nothing crosses another
// room's cells — so there is nothing a 2D grid buys that sequential
// concatenation doesn't already give for free.
type officeSeg struct {
	text  string
	style cellStyle
}

type officeLine []officeSeg

// runeWidth is the segment's visible width. ANSI wrapping is applied only at
// emit time, so this is always a true cell count under the repo's 1-rune =
// 1-cell discipline.
func (s officeSeg) runeWidth() int { return len([]rune(s.text)) }

func (l officeLine) width() int {
	w := 0
	for _, s := range l {
		w += s.runeWidth()
	}
	return w
}

// deskMarker is the office view's own status glyph set: shade blocks and
// plain ASCII, none of them East-Asian-Ambiguous or geometric (UI-7's ● ○ ×
// ◌ are off-limits here by contract). This keeps status legible even with
// colour off — the shade level alone reads as a liveness gradient, and the
// letter x for dead survives to grayscale, unlike a colour-only encoding.
func deskMarker(s vitals.Status) rune {
	switch s {
	case vitals.StatusActive:
		return '█'
	case vitals.StatusQuiet:
		return '▓'
	case vitals.StatusPending:
		return '▒'
	case vitals.StatusDead:
		return 'x'
	default:
		return '-'
	}
}

// deskInfo is one agent as it will be drawn: its name and the tile it paints.
type deskInfo struct {
	name string
	st   vitals.Status
}

// collectDesks walks n's subtree honouring Collapsed exactly as reflatten and
// buildGrid do: a collapsed node still appears (you hired it, it still draws
// a desk), but its children are invisible, so the walk doesn't descend.
func collectDesks(n *layout.Node, statuses map[string]vitals.Status) []deskInfo {
	if n == nil {
		return nil
	}
	out := []deskInfo{{name: n.ID, st: statuses[n.ID]}}
	if n.Collapsed {
		return out
	}
	for _, c := range n.Children {
		out = append(out, collectDesks(c, statuses)...)
	}
	return out
}

// deskTileText renders one desk as a fixed-width "[m name]" tile, m the
// status marker. Truncation (never overflow) beats a wider, size-varying
// tile: uniform desk width is what makes the room read as a grid of desks
// instead of a ragged list.
func deskTileText(d deskInfo) string {
	name := padTrunc(d.name, officeDeskNameW)
	return "[" + string(deskMarker(d.st)) + " " + name + "]"
}

// overflowTileText renders the "+N more" tile that absorbs desks beyond a
// room's grid cap. Styled neutral (styNode) at the call site: it names a
// count, not a status.
func overflowTileText(n int) string {
	return "[" + padTrunc(fmt.Sprintf("+%d more", n), officeDeskW-2) + "]"
}

// padTrunc rune-truncates s to at most n runes, then right-pads with spaces
// to exactly n. No ellipsis: a desk tile is small enough that a partial name
// plus "…" would just be a different partial name — the office contract
// wants ASCII/box glyphs only, and a hard cut reads as plainly as any marker.
func padTrunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	if len(r) < n {
		r = append(r, []rune(strings.Repeat(" ", n-len(r)))...)
	}
	return string(r)
}

// titleWall draws a box's top border with its title centered in ═ fill,
// truncating the title (never the box) when it's wider than inner — a box
// is never allowed to grow past the width budget its caller already settled
// on just to fit a label.
func titleWall(title string, inner int) officeLine {
	t := []rune(title)
	if len(t) > inner {
		t = t[:inner]
	}
	pad := inner - len(t)
	lp := pad / 2
	rp := pad - lp
	return officeLine{
		{text: "╔" + strings.Repeat("═", lp), style: styConnector},
		{text: string(t), style: styNone},
		{text: strings.Repeat("═", rp) + "╗", style: styConnector},
	}
}

// officeRoom is one department's rendered box: the desks that fit its grid
// cap, how many more exist (shown, not dropped, as a summary tile), and the
// box's own cell footprint.
type officeRoom struct {
	title    string // "name (total)"
	desks    []deskInfo
	overflow int
	cols     int
	rows     int
	width    int // full box width incl. walls
	height   int // full box height incl. walls (rows + 2)
}

// buildRoom sizes a department's box from its head node's whole subtree.
// Desks beyond the grid cap (officeMaxCols * officeMaxRows) collapse into one
// overflow tile rather than growing the room without bound — a department
// with 200 ICs must still fit on a floor plan.
func buildRoom(head *layout.Node, statuses map[string]vitals.Status) officeRoom {
	all := collectDesks(head, statuses)
	gridCap := officeMaxCols * officeMaxRows
	var shown []deskInfo
	overflow := 0
	if len(all) <= gridCap {
		shown = all
	} else {
		shown = all[:gridCap-1]
		overflow = len(all) - (gridCap - 1)
	}

	tiles := len(shown)
	if overflow > 0 {
		tiles++
	}
	cols := int(math.Ceil(math.Sqrt(float64(tiles))))
	if cols < 1 {
		cols = 1
	}
	if cols > officeMaxCols {
		cols = officeMaxCols
	}
	rows := (tiles + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}

	// The space-padded form is what actually gets drawn (titleWall centers it
	// in ═ fill), so the label carries its own padding from here on — sizing
	// and drawing then agree on one string instead of two callers each adding
	// their own spaces around a bare name.
	title := fmt.Sprintf(" %s (%d) ", head.ID, len(all))
	return sizeRoom(officeRoom{title: title, desks: shown, overflow: overflow, cols: cols, rows: rows})
}

// sizeRoom derives a room's box dimensions from its cols/rows and title —
// pulled out of buildRoom so shrinkRoomToWidth can re-derive width/height
// after lowering cols without duplicating the arithmetic.
func sizeRoom(r officeRoom) officeRoom {
	contentW := r.cols*officeDeskW + (r.cols-1)*officeDeskGapX
	boxW := contentW + 2*officeRoomPadX + 2
	titleW := len([]rune(r.title)) + 2 // "╔" + label + "╗"; ═ fill is a bonus, not guaranteed
	if titleW > boxW {
		boxW = titleW
	}
	r.width = boxW
	r.height = r.rows + 2
	return r
}

// shrinkRoomToWidth narrows a room that would otherwise blow the floor's
// width budget: first by adding desk rows (fewer columns, same desks), then
// — only if the title itself is what's too wide — by truncating the title
// text. Geometry always wins over legibility of the title text, never over
// the hard width bound (contract: never overflow).
func shrinkRoomToWidth(r officeRoom, maxW int) officeRoom {
	if r.width <= maxW {
		return r
	}
	for cols := r.cols; cols >= 1; cols-- {
		tiles := len(r.desks)
		if r.overflow > 0 {
			tiles++
		}
		rows := (tiles + cols - 1) / cols
		if rows < 1 {
			rows = 1
		}
		cand := sizeRoom(officeRoom{title: r.title, desks: r.desks, overflow: r.overflow, cols: cols, rows: rows})
		if cand.width <= maxW || cols == 1 {
			r = cand
			break
		}
	}
	if r.width > maxW {
		// The title alone still doesn't fit even at one column — truncate it.
		// budget accounts for the "╔"/"╗" corners (2 cells); the label already
		// carries its own space padding (see buildRoom).
		budget := maxW - 2
		if budget < 1 {
			budget = 1
		}
		title := []rune(r.title)
		if len(title) > budget {
			r.title = string(title[:budget])
		}
		r.width = maxW
	}
	return r
}

// packRooms bins rooms left-to-right into floor rows that each fit width,
// greedy and order-preserving — departments read left to right the same way
// the org chart does, never reshuffled for a tighter pack.
func packRooms(rooms []officeRoom, width int) [][]officeRoom {
	var floors [][]officeRoom
	var cur []officeRoom
	curW := 0
	for _, r := range rooms {
		add := r.width
		if len(cur) > 0 {
			add = curW + officeRoomGapX + r.width
		}
		if len(cur) > 0 && add > width {
			floors = append(floors, cur)
			cur = []officeRoom{r}
			curW = r.width
			continue
		}
		cur = append(cur, r)
		curW = add
	}
	if len(cur) > 0 {
		floors = append(floors, cur)
	}
	return floors
}

// renderOffice draws the org as a floor plan: the root is the executive suite
// at top, each of the root's direct children is a department "room"
// containing its whole subtree's agents as desks, coloured by live status.
// Returns a centered, width-bounded, newline-joined string (no trailing
// newline conventions beyond what the chart renderer uses). Returns "" for a
// nil root.
func renderOffice(root *layout.Node, statuses map[string]vitals.Status, width, height int) string {
	if root == nil {
		return ""
	}
	if width < 20 || height < 8 {
		return fitMessage("floor plan needs more room", width)
	}

	var lines []officeLine
	lines = append(lines, buildExecLines(root, statuses)...)
	lines = append(lines, officeLine{})

	var departments []*layout.Node
	if !root.Collapsed {
		departments = root.Children
	}

	if len(departments) == 0 {
		lines = append(lines, officeLine{{text: "(flat org — no departments yet)", style: styNode}})
	} else {
		floorLines, shown := buildFloorLines(departments, statuses, width, height)
		lines = append(lines, floorLines...)
		if shown < len(departments) {
			skipped := 0
			for _, d := range departments[shown:] {
				skipped += len(collectDesks(d, statuses))
			}
			lines = append(lines, officeLine{})
			lines = append(lines, officeLine{{
				text:  fmt.Sprintf("+%d more department(s) not shown (%d agents)", len(departments)-shown, skipped),
				style: styNode,
			}})
		}
	}

	lines = append(lines, officeLine{})
	lines = append(lines, legendLine())

	return joinOfficeLines(lines, width)
}

// fitMessage returns msg hard-truncated to width, or "" for a non-positive
// width — the one case where even a short message can't be shown.
func fitMessage(msg string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(msg)
	if len(r) > width {
		r = r[:width]
	}
	return string(r)
}

// buildExecLines draws the slim executive-suite room holding just the root.
func buildExecLines(root *layout.Node, statuses map[string]vitals.Status) []officeLine {
	label := " EXECUTIVE SUITE "
	deskW := officeDeskW + 2*officeRoomPadX + 2
	titleW := len([]rune(label)) + 2 // "╔" + label + "╗"
	boxW := max(deskW, titleW)
	inner := boxW - 2

	top := titleWall(label, inner)

	tile := deskTileText(deskInfo{name: root.ID, st: statuses[root.ID]})
	tileStyle := styleForStatus(statuses[root.ID])
	contentInner := inner - len([]rune(tile))
	lcPad := contentInner / 2
	rcPad := contentInner - lcPad
	content := officeLine{
		{text: "║" + strings.Repeat(" ", lcPad), style: styConnector},
		{text: tile, style: tileStyle},
		{text: strings.Repeat(" ", rcPad) + "║", style: styConnector},
	}

	bottom := officeLine{{text: "╚" + strings.Repeat("═", inner) + "╝", style: styConnector}}

	return []officeLine{top, content, bottom}
}

// buildFloorLines packs departments into floor rows, renders as many as fit
// height, and returns how many departments (out of the input, in order) made
// it onto the floor — the caller reports the rest as a count, never drops
// them silently.
func buildFloorLines(departments []*layout.Node, statuses map[string]vitals.Status, width, height int) (lines []officeLine, shown int) {
	rooms := make([]officeRoom, len(departments))
	for i, d := range departments {
		rooms[i] = shrinkRoomToWidth(buildRoom(d, statuses), width)
	}
	floors := packRooms(rooms, width)

	// Fixed overhead outside the floor: exec suite (3) + blank (1) + blank (1)
	// + legend (1) = 6, always present. Reserve 2 more for the "+N more
	// departments" notice (blank + text) renderOffice adds when this floor
	// can't seat everyone — reserving it pessimistically, whether or not it's
	// actually needed, is what keeps total output height <= height in every
	// case rather than just the common one.
	available := height - 8
	if available < 0 {
		available = 0
	}

	usedFloors := 0
	usedH := 0
	for _, floor := range floors {
		rowH := 0
		for _, r := range floor {
			rowH = max(rowH, r.height)
		}
		add := rowH
		if usedFloors > 0 {
			add += officeRoomGapY
		}
		if usedH+add > available {
			break
		}
		usedH += add
		usedFloors++
		shown += len(floor)
	}

	for i, floor := range floors[:usedFloors] {
		if i > 0 {
			lines = append(lines, officeLine{})
		}
		rowH := 0
		for _, r := range floor {
			rowH = max(rowH, r.height)
		}
		lines = append(lines, renderFloorRow(floor, rowH)...)
	}
	return lines, shown
}

// renderFloorRow draws a set of rooms side by side, all padded to targetH so
// their bottom walls line up — the "looking down at a floor" effect breaks
// immediately if one room's floor is a row lower than its neighbour's.
func renderFloorRow(rooms []officeRoom, targetH int) []officeLine {
	perRoom := make([][]officeLine, len(rooms))
	for i, r := range rooms {
		perRoom[i] = renderRoomBox(r, targetH-2)
	}
	out := make([]officeLine, targetH)
	for y := 0; y < targetH; y++ {
		var line officeLine
		for i := range rooms {
			if i > 0 {
				line = append(line, officeSeg{text: strings.Repeat(" ", officeRoomGapX)})
			}
			line = append(line, perRoom[i][y]...)
		}
		out[y] = line
	}
	return out
}

// renderRoomBox draws one room's box at exactly targetRows desk rows,
// padding with blank interior rows when the room's own content is shorter —
// see renderFloorRow.
func renderRoomBox(r officeRoom, targetRows int) []officeLine {
	inner := r.width - 2
	top := titleWall(r.title, inner) // r.title already carries its space padding (buildRoom)

	tiles := make([]officeSeg, 0, len(r.desks)+1)
	for _, d := range r.desks {
		tiles = append(tiles, officeSeg{text: deskTileText(d), style: styleForStatus(d.st)})
	}
	if r.overflow > 0 {
		tiles = append(tiles, officeSeg{text: overflowTileText(r.overflow), style: styNode})
	}

	lines := make([]officeLine, 0, targetRows+2)
	lines = append(lines, top)
	for row := 0; row < targetRows; row++ {
		var rowTiles []officeSeg
		for col := 0; col < r.cols; col++ {
			idx := row*r.cols + col
			if col > 0 {
				rowTiles = append(rowTiles, officeSeg{text: strings.Repeat(" ", officeDeskGapX)})
			}
			if row < r.rows && idx < len(tiles) {
				rowTiles = append(rowTiles, tiles[idx])
			} else {
				rowTiles = append(rowTiles, officeSeg{text: strings.Repeat(" ", officeDeskW)})
			}
		}
		rowW := 0
		for _, seg := range rowTiles {
			rowW += seg.runeWidth()
		}
		pad := inner - 2*officeRoomPadX - rowW
		if pad < 0 {
			pad = 0
		}
		content := officeLine{{text: "║" + strings.Repeat(" ", officeRoomPadX), style: styConnector}}
		content = append(content, rowTiles...)
		content = append(content, officeSeg{text: strings.Repeat(" ", pad+officeRoomPadX) + "║", style: styConnector})
		lines = append(lines, content)
	}
	lines = append(lines, officeLine{{text: "╚" + strings.Repeat("═", inner) + "╝", style: styConnector}})
	return lines
}

// legendLine maps each status marker to its word, ASCII/box-safe so it never
// trips the same ambiguous-width hazard the markers themselves avoid.
func legendLine() officeLine {
	order := []struct {
		st    vitals.Status
		label string
	}{
		{vitals.StatusActive, "active"},
		{vitals.StatusQuiet, "quiet"},
		{vitals.StatusPending, "pending"},
		{vitals.StatusDead, "dead"},
	}
	var line officeLine
	for i, e := range order {
		if i > 0 {
			line = append(line, officeSeg{text: "   "})
		}
		line = append(line, officeSeg{text: string(deskMarker(e.st)), style: styleForStatus(e.st)})
		line = append(line, officeSeg{text: " " + e.label})
	}
	return line
}

// joinOfficeLines centers every line on the widest one and emits ANSI runs
// per segment, mirroring emitStyledRow's contiguous-span approach — colour
// never drifts from geometry because the geometry (the segment text) and the
// colour (the segment style) are written together, in one pass, from the
// same value.
func joinOfficeLines(lines []officeLine, width int) string {
	// Defensive hard clamp: every construction path above already respects
	// width, but a single guaranteed truncation point here is what makes
	// "never overflow" (contract §4) an invariant instead of a hope.
	for i, l := range lines {
		lines[i] = clampLine(l, width)
	}

	w := 0
	for _, l := range lines {
		w = max(w, l.width())
	}
	prefix := centerPad(width, w)

	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		trimmed := trimTrailingBlank(l)
		if len(trimmed) == 0 {
			continue
		}
		b.WriteString(prefix)
		for _, seg := range trimmed {
			if seg.text == "" {
				continue
			}
			if colorEnabled {
				if code := ansiFor(seg.style); code != "" {
					b.WriteString(code)
					b.WriteString(seg.text)
					b.WriteString(ansiReset)
					continue
				}
			}
			b.WriteString(seg.text)
		}
	}
	return b.String()
}

// clampLine hard-truncates a line to at most width visible runes, cutting
// whole or partial segments from the tail. Segment order/style up to the cut
// point is preserved.
func clampLine(l officeLine, width int) officeLine {
	if width < 0 {
		return nil
	}
	out := make(officeLine, 0, len(l))
	remaining := width
	for _, seg := range l {
		if remaining <= 0 {
			break
		}
		r := []rune(seg.text)
		if len(r) > remaining {
			r = r[:remaining]
		}
		out = append(out, officeSeg{text: string(r), style: seg.style})
		remaining -= len(r)
	}
	return out
}

// trimTrailingBlank drops trailing all-space segments, matching
// emitStyledRow's trailing-blank trim so office lines don't carry dead
// whitespace into the terminal.
func trimTrailingBlank(l officeLine) officeLine {
	end := len(l)
	for end > 0 && strings.TrimRight(l[end-1].text, " ") == "" {
		end--
	}
	if end == 0 {
		return nil
	}
	out := make(officeLine, end)
	copy(out, l[:end])
	last := out[end-1]
	last.text = strings.TrimRight(last.text, " ")
	out[end-1] = last
	return out
}
