package ui

import (
	"strings"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

// Newswire + pulse monitor (F6). The old ticker showed a single most-recent
// message keyed by raw peer id ("demo-3 → demo-1"). This turns it into a
// broadcast-style news band that scrolls the recent feed by AGENT NAME, plus a
// heartbeat strip that shows real message activity over time. Both are honest:
// they render real broker rows and real timestamps, and nothing is synthesised —
// a quiet company scrolls nothing and flatlines, which is the truth.

// newswireMaxItems bounds how many recent messages ride the band.
const newswireMaxItems = 10

// newswireBand builds the scrolling band from the most recent messages, oldest
// of the window first so it reads left-to-right as it scrolls. nameOf resolves a
// peer id to an agent name (falling back to the id when unknown, e.g. a peer
// with no node). Returns "" for no messages.
func newswireBand(msgs []broker.Message, nameOf func(string) string, maxItems int) string {
	if len(msgs) == 0 {
		return ""
	}
	start := len(msgs) - maxItems
	if start < 0 {
		start = 0
	}
	parts := make([]string, 0, len(msgs)-start)
	for _, m := range msgs[start:] {
		// ASCII arrow and quotes only — keep the band in the single-width class.
		parts = append(parts, nameOf(m.FromID)+" -> "+nameOf(m.ToID)+`: "`+oneLine(m.Text)+`"`)
	}
	return strings.Join(parts, "   -   ")
}

// marqueeWindow returns a width-rune slice of band scrolled by offset, wrapping
// seamlessly (a gap is appended so the tail doesn't jam against the head). A band
// shorter than width is returned as-is (nothing to scroll). Rune-based so multi-
// byte text is never split mid-character and the width is a true cell count.
func marqueeWindow(band string, width, offset int) string {
	if width <= 0 || band == "" {
		return ""
	}
	r := []rune(band)
	if len(r) <= width {
		return band
	}
	loop := append(r, []rune("     ")...) // gap between end and restart
	n := len(loop)
	off := offset % n
	if off < 0 {
		off += n
	}
	out := make([]rune, width)
	for i := 0; i < width; i++ {
		out[i] = loop[(off+i)%n]
	}
	return string(out)
}

// pulseChars are unambiguous single-width block elements (unlike the geometric
// glyphs), so the heartbeat strip never shears the layout.
var pulseChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// pulseMonitor renders a scrolling heartbeat: each column is a short time bucket
// ending at now (rightmost = newest), its height set by how many messages landed
// in that slice. As now advances every frame the whole strip shifts left, so a
// burst of traffic reads as a spike travelling away into history. width columns
// of `bucket` each cover the last width*bucket of time. Empty history flatlines.
func pulseMonitor(msgs []broker.Message, now time.Time, width int, bucket time.Duration) string {
	if width <= 0 || bucket <= 0 {
		return ""
	}
	counts := make([]int, width)
	for _, m := range msgs {
		t, ok := parseMsgTime(m.SentAt)
		if !ok || now.Before(t) {
			continue
		}
		// Column 0 is the oldest visible slice, width-1 is "now".
		slot := width - 1 - int(now.Sub(t)/bucket)
		if slot >= 0 && slot < width {
			counts[slot]++
		}
	}
	maxv := 0
	for _, c := range counts {
		if c > maxv {
			maxv = c
		}
	}
	var b strings.Builder
	for _, c := range counts {
		if maxv == 0 {
			b.WriteRune(pulseChars[0])
			continue
		}
		b.WriteRune(pulseChars[c*(len(pulseChars)-1)/maxv])
	}
	return b.String()
}

// oneLine flattens whitespace/newlines in message text to a single spaced line so
// one message can't break the band across rows.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// renderNewswire draws the two-line broadcast strip: a scrolling news band of
// the recent feed, and a heartbeat pulse of message activity beneath it. Returns
// "" when there's no traffic (a quiet company shows nothing, honestly). The band
// scrolls only when motion is on; motion off shows a static, readable window.
func (m Model) renderNewswire() string {
	if m.live == nil || m.live.newswire == "" {
		return ""
	}
	bandW := m.width - 4
	if bandW < 10 {
		bandW = 10
	}
	offset := 0
	if colorEnabled && m.motion.animates() {
		offset = m.frame / 2 // ~5 cells/sec at 10fps — a readable marquee
	}
	band := marqueeWindow(m.live.newswire, bandW, offset)

	pw := bandW
	if pw > 60 {
		pw = 60
	}
	pulse := pulseMonitor(m.live.msgs, time.Now(), pw, FrameInterval)

	var b strings.Builder
	b.WriteString("\n  " + wrapANSI("▎", styActive) + " " + band + "\n")
	b.WriteString("  " + wrapANSI("▎", styActive) + " " + wrapANSI(pulse, styActive) + "\n")
	return b.String()
}

// wrapANSI wraps s in the current theme's escape for style s when colour is on.
func wrapANSI(s string, style cellStyle) string {
	if !colorEnabled {
		return s
	}
	if a := ansiFor(style); a != "" {
		return a + s + ansiReset
	}
	return s
}

// peerName resolves a peer id to its agent name, falling back to the raw id when
// the peer has no node in this company (e.g. another company's session that
// messaged in). The fallback is honest — an unknown sender shows as its id rather
// than a fabricated name.
func (m Model) peerName(peerID string) string {
	if m.live != nil {
		if n, ok := m.live.peerToName[peerID]; ok {
			return n
		}
	}
	return peerID
}
