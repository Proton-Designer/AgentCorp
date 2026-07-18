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

// palette maps each cellStyle to a 256-colour SGR foreground code (as a bare
// number; sgr() wraps it). Index 0 (styNone) is always empty. 256-colour, not
// 24-bit truecolor, because macOS Terminal.app — a likely host — supports 256
// but not truecolor, while every modern terminal and tmux supports 256 without
// extra config.
type palette [7]string

// theme is a named palette. Cycling themes ('t') is pure cosmetics — a big lever
// on how "the whole thing feels" for near-zero code, exactly the kind of thing
// this project's whole frontend emphasis is about.
type theme struct {
	name string
	p    palette
}

// themes are ordered; 't' advances through them. Index by cellStyle:
// [none, connector, active, quiet, pending, dead, node].
var themes = []theme{
	{"mint", palette{"", "240", "48", "79", "214", "241", "122"}},
	{"matrix", palette{"", "22", "46", "34", "226", "238", "40"}},
	{"cyber", palette{"", "54", "51", "45", "201", "240", "141"}},
	{"amber", palette{"", "94", "214", "178", "220", "240", "208"}},
	{"ice", palette{"", "24", "51", "39", "123", "240", "45"}},
}

// currentTheme indexes themes. A package var (not per-model) because a single
// TUI process has one active palette; the theme test saves/restores it.
var currentTheme int

// cycleTheme advances to the next palette and returns its name.
func cycleTheme() string {
	currentTheme = (currentTheme + 1) % len(themes)
	return themes[currentTheme].name
}

// ansiFor maps a cellStyle to the current theme's SGR foreground escape, or ""
// for no styling.
func ansiFor(s cellStyle) string {
	if int(s) < 0 || int(s) >= len(themes[currentTheme].p) {
		return ""
	}
	code := themes[currentTheme].p[s]
	if code == "" {
		return ""
	}
	return "\x1b[38;5;" + code + "m"
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
