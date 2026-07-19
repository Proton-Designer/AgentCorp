package ui

import (
	"fmt"
	"strings"

	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// Mission-control view (F5). A command-center dashboard: the org chart in a big
// left panel, and a right column of stacked panels — VITALS, WIRE, ALERTS. It's
// a view mode (like the office), an alternate use of the whole viewport rather
// than a layer, because it re-budgets the screen into panes. Everything it shows
// is real: the same live status, the same broker feed, the same dead/stale
// signals the chart derives — just arranged as a control deck.

// panelBox draws a titled bordered box of exactly w×h cells, its content clipped
// and padded into the interior. Content lines may already carry ANSI (e.g. the
// embedded chart); they're padded by visible width so colour never throws off the
// box geometry. Returns h lines, each visibleWidth == w.
func panelBox(title string, content []string, w, h int, border cellStyle) []string {
	if w < 4 || h < 2 {
		return nil
	}
	inner := w - 2
	lead := "─ " + title + " "
	if lr := []rune(lead); len(lr) > inner {
		lead = string(lr[:inner])
	}
	rule := lead + strings.Repeat("─", inner-len([]rune(lead)))

	out := make([]string, 0, h)
	out = append(out, wrapANSI("╭"+rule+"╮", border))
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		out = append(out, wrapANSI("│", border)+padVisible(line, inner)+wrapANSI("│", border))
	}
	out = append(out, wrapANSI("╰"+strings.Repeat("─", inner)+"╯", border))
	return out
}

// padVisible pads s to w cells by visible width. Plain text longer than w is
// rune-clipped; ANSI-bearing text (already sized by its producer to fit) is left
// intact rather than risk cutting mid-escape.
func padVisible(s string, w int) string {
	vw := visibleWidth(s)
	if vw >= w {
		if !strings.Contains(s, "\x1b") {
			return string([]rune(s)[:w])
		}
		return s
	}
	return s + strings.Repeat(" ", w-vw)
}

// zipColumns places columns side by side with a one-space gutter, padding shorter
// columns with blank cells so the rows stay aligned.
func zipColumns(cols [][]string, widths []int) []string {
	h := 0
	for _, c := range cols {
		if len(c) > h {
			h = len(c)
		}
	}
	out := make([]string, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for i, c := range cols {
			if i > 0 {
				b.WriteByte(' ')
			}
			if y < len(c) {
				b.WriteString(c[y])
			} else {
				b.WriteString(strings.Repeat(" ", widths[i]))
			}
		}
		out[y] = b.String()
	}
	return out
}

// renderMission composes the dashboard. It bounds itself to width×height and
// degrades to a message when the terminal is too small to hold two columns.
func (m Model) renderMission(width, height int) string {
	if width < 50 || height < 12 {
		return fitMessage("mission control needs a larger terminal", width)
	}
	// Reserve the 2-cell left indent every row carries, so the right panel's wall
	// lands inside the terminal rather than one cell past it.
	inner := width - 2
	leftW := inner * 3 / 5
	rightW := inner - leftW - 1

	// Left: the styled org chart, rendered at the panel's interior width.
	statuses := m.statusMap()
	chart := RenderStyled(m.root, leftW-2, func(id string) vitals.Status { return statuses[id] })
	chartLines := strings.Split(chart, "\n")
	left := panelBox("ORG", chartLines, leftW, height, styConnector)

	// Right column: VITALS + WIRE + ALERTS, heights summing to `height`.
	vitalsH := 4
	alertsH := 5
	wireH := height - vitalsH - alertsH
	if wireH < 3 {
		wireH = 3
	}
	right := make([]string, 0, height)
	right = append(right, panelBox("VITALS", m.missionVitals(), rightW, vitalsH, styConnector)...)
	right = append(right, panelBox("WIRE", m.missionWire(rightW-2, wireH-2), rightW, wireH, styConnector)...)
	right = append(right, panelBox("ALERTS", m.missionAlerts(rightW-2), rightW, alertsH, styConnector)...)

	rows := zipColumns([][]string{left, right}, []int{leftW, rightW})

	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("  " + r)
	}
	return b.String()
}

// missionVitals is the at-a-glance readout: counts + a throughput spark.
func (m Model) missionVitals() []string {
	s := m.live.summary
	return []string{
		wrapANSI(fmt.Sprintf("%d agents", s.Alive), styActive) +
			fmt.Sprintf("   %d active  %d quiet", s.Active, s.Quiet),
		"throughput " + m.live.spark,
	}
}

// missionWire is the recent feed as a list (newest last), resolved to names.
func (m Model) missionWire(w, n int) []string {
	msgs := m.live.msgs
	start := len(msgs) - n
	if start < 0 {
		start = 0
	}
	out := make([]string, 0, n)
	for _, msg := range msgs[start:] {
		line := m.peerName(msg.FromID) + " -> " + m.peerName(msg.ToID) + ": " + oneLine(msg.Text)
		out = append(out, fitRunes(line, w))
	}
	return out
}

// missionAlerts surfaces what needs attention: dead agents, staleness, and
// unmanaged sessions. "all clear" when there's nothing — honestly, not a blank.
func (m Model) missionAlerts(w int) []string {
	var out []string
	var dead []string
	for name, st := range m.statusMap() {
		if st == vitals.StatusDead {
			dead = append(dead, name)
		}
	}
	if len(dead) > 0 {
		out = append(out, wrapANSI(fitRunes(fmt.Sprintf("x %d dead: %s", len(dead), strings.Join(dead, ", ")), w), styDead))
	}
	if m.live.stale {
		out = append(out, wrapANSI(fitRunes("! STALE — poll failed", w), styPending))
	}
	if u := m.live.summary.Unmanaged; u > 0 {
		out = append(out, fitRunes(fmt.Sprintf("%d unmanaged session(s)", u), w))
	}
	if len(out) == 0 {
		out = append(out, wrapANSI("all clear", styActive))
	}
	return out
}
