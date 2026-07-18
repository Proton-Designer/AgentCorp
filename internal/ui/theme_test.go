package ui

import (
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 't' cycles themes and changes the active palette; every theme defines a full,
// distinct set of status colours.
func TestCycleThemeChangesPalette(t *testing.T) {
	saved := currentTheme
	defer func() { currentTheme = saved }()

	currentTheme = 0
	before := ansiFor(styActive)
	name := cycleTheme()
	after := ansiFor(styActive)

	if name == "" {
		t.Fatal("cycleTheme returned no name")
	}
	if before == after {
		t.Fatalf("theme cycle did not change the active colour (%q)", before)
	}
	// One full lap (len(themes) advances from 0) returns to the start.
	for i := 1; i < len(themes); i++ {
		cycleTheme()
	}
	if currentTheme != 0 {
		t.Fatalf("cycling a full lap did not return to theme 0, at %d", currentTheme)
	}
}

func TestEveryThemeDefinesAllStatusColors(t *testing.T) {
	saved := currentTheme
	defer func() { currentTheme = saved }()
	for i := range themes {
		currentTheme = i
		for _, s := range []cellStyle{styConnector, styActive, styQuiet, styPending, styDead, styNode} {
			if ansiFor(s) == "" {
				t.Fatalf("theme %q missing a colour for style %d", themes[i].name, s)
			}
		}
	}
}

// The 't' key updates the flash to name the new theme.
func TestThemeKeyFlashes(t *testing.T) {
	saved := currentTheme
	defer func() { currentTheme = saved }()
	old := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = old }()

	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m = send(m, "t")
	if m.flash == "" || m.flash[:6] != "theme:" {
		t.Fatalf("t did not flash the theme name, got %q", m.flash)
	}
}
