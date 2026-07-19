package ui

import (
	"strings"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/anim"
	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// This file is the animation render path. Its cardinal rule, encoded here rather
// than trusted to memory: the overlay may only REPAINT or RE-GLYPH cells the
// cached base already positioned — it never runs layout, and it never invents a
// signal. Every animated cell traces back to a real substrate fact (an active
// node, a real broker message). anim/ supplies the wave *shape* as a function of
// an int frame; it cannot originate activity. That is what keeps the pretty
// version as honest as the plain one.

// ovCell is one animated cell: an optional replacement rune (0 keeps the base
// grid's rune) and an optional explicit ANSI escape ("" keeps the base cell's
// themed colour).
type ovCell struct {
	r    rune
	ansi string
}

// overlay is a sparse set of animated cells, keyed by [x,y]. Sparse because only
// a handful of cells move per frame; the base grid carries everything else.
type overlay map[[2]int]ovCell

// ledBreathPeriod is the frame count of one status-LED breath at FrameInterval
// (~10fps), so ~1.6s per cycle — a resting-heart tempo, not a strobe.
const ledBreathPeriod = 16

// ledBreathLevels quantises the breath into this many intensity buckets. Kept
// small on purpose: a card's rendered bytes change only when the breath crosses
// a bucket boundary, so most frames leave the row untouched and Bubble Tea's
// per-line diff still skips it.
const ledBreathLevels = 3

// ensureBase (re)builds the cached rune+style grid, but only when geometry or
// per-node status could have changed — tracked by baseVersion, bumped on a data
// tick and on a fold. Between those the 100ms frame path reuses this cache and
// never re-runs Reingold-Tilford. It writes through m.live (a pointer), so a
// value-receiver View can populate the cache without the model needing to be
// addressable.
func (m Model) ensureBase() {
	ls := m.live
	if ls == nil {
		return
	}
	if ls.baseGrid != nil && ls.baseBuiltVer == ls.baseVersion {
		return
	}
	statuses := m.statusMap()
	grid, styles, w, h := buildGrid(m.root,
		func(id string) cellStyle { return styleForStatus(statuses[id]) },
		func(id string) rune { return statusGlyph(statuses[id]) })
	ls.baseGrid, ls.baseStyles, ls.baseW, ls.baseH = grid, styles, w, h
	ls.baseBuiltVer = ls.baseVersion
}

// buildOverlay assembles this frame's animated cells from the cached base. F0
// draws one honest effect: the status LED of every ACTIVE node breathes. Active
// is the already-vetted message-recency signal (vitals.StatusActive) — the pulse
// borrows it wholesale and adds nothing, so it can never imply life the substrate
// didn't report.
func (m Model) buildOverlay() overlay {
	ov := overlay{}
	if m.live == nil || !m.motion.animates() || m.root == nil {
		return ov
	}
	statuses := m.statusMap()
	lvl := anim.Level(anim.Pulse(m.frame, ledBreathPeriod), ledBreathLevels)
	breath := dimANSI(styActive, lvl)
	if breath == "" {
		return ov
	}
	byName := map[string]*layout.Node{}
	var walk func(n *layout.Node)
	walk = func(n *layout.Node) {
		byName[n.ID] = n
		if statuses[n.ID] == vitals.StatusActive && n.W >= 4 {
			// Lively mode: the whole card border breathes, phase-staggered per card
			// so they don't pulse in unison (organic, and it spreads the byte
			// changes across frames so the per-line diff still skips most rows).
			if m.motion == motionLively {
				addCardBreath(ov, n, m.frame)
			}
			// The LED sits at (x+1, y) — the border cell just after the corner,
			// exactly where drawCard paints the status glyph. Set last so its own
			// (unstaggered) breath owns that cell. Rune stays the base's ●.
			ov[[2]int{n.X + 1, n.Y}] = ovCell{ansi: breath}
		}
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.root)

	m.addFlowOverlay(ov, byName)
	return ov
}

// addCardBreath paints an active card's whole border at a breathing intensity,
// phase-offset by the card's position so the org doesn't pulse as one block. Hue
// is unchanged — only the active colour's brightness moves — so it reads as a
// vital sign, never as a status change.
func addCardBreath(ov overlay, n *layout.Node, frame int) {
	off := (n.X*3 + n.Y*7) % ledBreathPeriod
	lvl := anim.Level(anim.Pulse(frame+off, ledBreathPeriod), ledBreathLevels)
	a := dimANSI(styActive, lvl)
	if a == "" {
		return
	}
	x, y, w, h := n.X, n.Y, n.W, n.H
	for i := 0; i < w; i++ {
		ov[[2]int{x + i, y}] = ovCell{ansi: a}
		ov[[2]int{x + i, y + h - 1}] = ovCell{ansi: a}
	}
	for j := 0; j < h; j++ {
		ov[[2]int{x, y + j}] = ovCell{ansi: a}
		ov[[2]int{x + w - 1, y + j}] = ovCell{ansi: a}
	}
}

// addFlowOverlay paints each in-flight message as a bright comet riding its
// connector wire (F1). Only edges with a real recent broker row (m.live.flows,
// derived at the data tick) are drawn, and each pulse stops at the wire's end —
// it never enters a card, so transport is never mistaken for the destination
// acting on the message.
func (m Model) addFlowOverlay(ov overlay, byName map[string]*layout.Node) {
	if len(m.live.flows) == 0 {
		return
	}
	now := time.Now()
	for _, f := range m.live.flows {
		if age := now.Sub(f.sentAt); age < 0 || age > FlowWindow {
			continue // aged out of the window since the last data tick
		}
		p, c := byName[f.parent], byName[f.child]
		if p == nil || c == nil {
			continue
		}
		path := edgePath(p, c)
		if len(path) == 0 {
			continue
		}
		head := anim.Along(m.frame, flowFramePeriod, len(path))
		for tail := 0; tail <= flowTail; tail++ {
			idx := head - tail
			if idx < 0 {
				break
			}
			cell := path[idx]
			if f.up { // travels child→parent: ride the path from the child end
				cell = path[len(path)-1-idx]
			}
			// Head brightest, trailing cells fade — a comet along the wire. Don't
			// overwrite a card/LED cell the base already owns at full status colour.
			if a := dimANSI(styActive, 2-tail); a != "" {
				ov[cell] = ovCell{ansi: a}
			}
		}
	}
}

// renderAnimated is the motion-on chart path: composite this frame's overlay over
// the cached base and emit. When the overlay is empty (nothing moving this
// frame), it defers to the plain styled emit so a still frame is byte-identical
// to RenderStyled — motion can never make a calm chart differ from the static one.
func (m Model) renderAnimated() string {
	m.ensureBase()
	ls := m.live
	if ls.baseW < 1 || ls.baseH < 1 {
		return ""
	}
	ov := m.buildOverlay()
	if len(ov) == 0 {
		return m.emitStyledBase()
	}
	prefix := centerPad(m.width, ls.baseW)
	var b strings.Builder
	for y := 0; y < ls.baseH; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		emitOverlayRow(&b, ls.baseGrid[y], ls.baseStyles[y], ov, y, prefix)
	}
	return b.String()
}

// emitStyledBase emits the cached base with no overlay, matching RenderStyled's
// output exactly (same buildGrid product, same emitStyledRow, same prefix). This
// is the still-frame path.
func (m Model) emitStyledBase() string {
	ls := m.live
	prefix := centerPad(m.width, ls.baseW)
	var b strings.Builder
	for i := 0; i < ls.baseH; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		emitStyledRow(&b, ls.baseGrid[i], ls.baseStyles[i], prefix)
	}
	return b.String()
}

// emitOverlayRow writes one row, resolving each cell against the overlay: an
// overlay rune (if non-zero) replaces the base rune, an overlay ANSI (if set)
// replaces the base cell's themed colour. Runs of identical resolved-ANSI share
// one escape, closed with a reset — same run-length shape as emitStyledRow, but
// keyed on the resolved escape string so an overlay's brightness attributes group
// correctly. Trailing blanks are trimmed, but the row is extended rightward to
// include any overlay glyph placed past the base content (a particle in the
// margin).
func emitOverlayRow(b *strings.Builder, runes []rune, styles []cellStyle, ov overlay, y int, prefix string) {
	end := len(runes)
	for end > 0 && runes[end-1] == ' ' {
		end--
	}
	for key, c := range ov {
		if key[1] == y && c.r != 0 && key[0] >= end {
			end = key[0] + 1
		}
	}
	if end == 0 {
		return
	}
	b.WriteString(prefix)
	cur := ""
	open := false
	for x := 0; x < end; x++ {
		r := ' '
		if x < len(runes) {
			r = runes[x]
		}
		base := styNone
		if x < len(styles) {
			base = styles[x]
		}
		ansi := ansiFor(base)
		if c, ok := ov[[2]int{x, y}]; ok {
			if c.r != 0 {
				r = c.r
			}
			if c.ansi != "" {
				ansi = c.ansi
			}
		}
		if ansi != cur {
			if open {
				b.WriteString(ansiReset)
				open = false
			}
			if ansi != "" {
				b.WriteString(ansi)
				open = true
			}
			cur = ansi
		}
		b.WriteRune(r)
	}
	if open {
		b.WriteString(ansiReset)
	}
}
