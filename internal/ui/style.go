package ui

import (
	"os"

	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// colorEnabled gates ANSI colour output, honouring the NO_COLOR convention
// (https://no-color.org). It is a var, not a const, so tests can force a
// deterministic value and so a future settings toggle can flip it at runtime.
var colorEnabled = os.Getenv("NO_COLOR") == ""

// cellStyle is a per-grid-cell paint bucket. The renderer builds a parallel
// grid of these alongside the rune grid, then emits one ANSI run per contiguous
// same-style span — so colour is a second, optional layer over the exact same
// geometry the monochrome renderer produces (which keeps the layout tests, and
// the plain code path, untouched).
type cellStyle uint8

const (
	styNone      cellStyle = iota // empty cell or plain default — no ANSI
	styConnector                  // the edges between cards
	styActive                     // a bound agent that spoke recently
	styQuiet                      // a bound agent, alive but quiet
	styPending                    // a hire still forming (no peer yet)
	styDead                       // tombstoned or its peer vanished
	styNode                       // a node whose status is unknown/default
)

// ansiFor maps a cellStyle to a 256-colour SGR foreground escape, or "" for no
// styling. 256-colour (not 24-bit truecolor) because macOS Terminal.app — a
// likely host — supports 256 but not truecolor, and every modern terminal and
// tmux supports 256 without extra config.
func ansiFor(s cellStyle) string {
	switch s {
	case styConnector:
		return "\x1b[38;5;240m" // dim grey — present but recedes
	case styActive:
		return "\x1b[38;5;48m" // bright green — talking now
	case styQuiet:
		return "\x1b[38;5;79m" // calm aquamarine — alive, resting
	case styPending:
		return "\x1b[38;5;214m" // amber — forming
	case styDead:
		return "\x1b[38;5;241m" // faded grey — gone
	case styNode:
		return "\x1b[38;5;122m" // mint — the house colour
	default:
		return ""
	}
}

const ansiReset = "\x1b[0m"

// styleForStatus maps a live vitals.Status to the card colour it paints with.
func styleForStatus(s vitals.Status) cellStyle {
	switch s {
	case vitals.StatusActive:
		return styActive
	case vitals.StatusQuiet:
		return styQuiet
	case vitals.StatusPending:
		return styPending
	case vitals.StatusDead:
		return styDead
	default:
		return styNode
	}
}
