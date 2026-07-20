package main

import (
	"os"
	"os/exec"
	"strings"
)

// setupTmuxInput turns on the two tmux options click-to-open needs — `mouse` (so
// tmux forwards clicks to AgentCorp) and `focus-events` (so AgentCorp is told when
// it regains focus, and can re-arm the mouse a terminal may have dropped) — and
// returns a function that restores their previous values on exit.
//
// This is the "zero config" path: rather than asking every user to edit
// ~/.tmux.conf, AgentCorp sets what it needs itself. Two properties keep it polite:
// the options are set SESSION-scoped (no -g), so the user's global config and other
// sessions are untouched; and the previous values are captured and restored when
// AgentCorp exits, so the terminal is left exactly as it was found. It is a no-op
// outside tmux — click-to-open needs tmux anyway, and the keyboard path still works.
func setupTmuxInput() func() {
	if os.Getenv("TMUX") == "" {
		return func() {}
	}
	prevMouse := tmuxGet("mouse")
	prevFocus := tmuxGet("focus-events")
	tmuxSet("mouse", "on")
	tmuxSet("focus-events", "on")
	return func() {
		if prevMouse != "" {
			tmuxSet("mouse", prevMouse)
		}
		if prevFocus != "" {
			tmuxSet("focus-events", prevFocus)
		}
	}
}

// tmuxGet reads a session option's effective value, or "" if it can't be read.
func tmuxGet(opt string) string {
	out, err := exec.Command("tmux", "show-options", "-v", opt).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// tmuxSet sets a session option (best-effort; a failure just means click-to-open
// falls back to the keyboard path).
func tmuxSet(opt, val string) {
	_ = exec.Command("tmux", "set-option", opt, val).Run()
}
