package ui

import "strings"

// helpKeys is the full keymap, grouped, shown by the '?' overlay. Kept here as
// the single source of truth so the overlay can't drift from the footer hint.
var helpKeys = [][2]string{
	{"↑↓ / j k", "move the cursor"},
	{"click / enter", "open the agent's session (switch to its tmux window)"},
	{"space", "fold / unfold a subtree"},
	{"i", "inspect the selected agent"},
	{"h", "hire a new agent (then pick a role)"},
	{"a", "adopt an unmanaged session"},
	{"m", "message the selected agent"},
	{"b", "broadcast to its whole team"},
	{"R", "rename the selected agent"},
	{"r", "move it under a new manager"},
	{"x", "fire the selected agent"},
	{"z", "revive a dead agent (resume its session)"},
	{"shift-Z", "revive ALL dead agents at once"},
	{"shift-S", "arm/disarm auto-supervision (self-healing)"},
	{"shift-D", "disband a subtree"},
	{"/", "find by name / role / status"},
	{"l", "activity feed (org message log)"},
	{"o", "toggle office / floor-plan view"},
	{"g", "toggle mission-control dashboard"},
	{"e", "export org snapshot (JSON + Markdown)"},
	{"t", "cycle colour theme"},
	{"v", "cycle motion (off / calm / lively)"},
	{"?", "this help"},
	{"q", "quit"},
}

// renderHelp draws the keybind reference plus a colour legend that names what
// each status colour means — so the palette is self-documenting, not folklore.
func (m Model) renderHelp() string {
	var b strings.Builder
	b.WriteString("  keys\n")
	for _, k := range helpKeys {
		b.WriteString("  " + padRight(k[0], 10) + " " + k[1] + "\n")
	}
	b.WriteString("\n  status colours\n")
	b.WriteString("  " + swatch(styActive, "active") + "   " +
		swatch(styQuiet, "quiet") + "   " +
		swatch(styPending, "pending") + "   " +
		swatch(styDead, "dead") + "\n")
	b.WriteString("  esc / ? close\n")
	return b.String()
}

// swatch renders a colored bullet + label for the legend.
func swatch(s cellStyle, label string) string {
	dot := "●"
	if colorEnabled && ansiFor(s) != "" {
		dot = ansiFor(s) + "●" + ansiReset
	}
	return dot + " " + label
}

// padRight pads s with spaces to at least n runes.
func padRight(s string, n int) string {
	if r := []rune(s); len(r) < n {
		return s + strings.Repeat(" ", n-len(r))
	}
	return s
}
