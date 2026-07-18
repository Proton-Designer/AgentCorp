package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// '?' opens the help overlay listing keys and the colour legend; esc closes it.
func TestHelpOverlay(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m = send(m, "?")
	if m.mode != modeHelp {
		t.Fatalf("? did not open help (mode=%v)", m.mode)
	}
	v := m.View()
	for _, want := range []string{"hire", "adopt", "broadcast", "status colours", "active", "dead"} {
		if !strings.Contains(v, want) {
			t.Fatalf("help overlay missing %q:\n%s", want, v)
		}
	}
	nm, _ := m.Update(key("esc"))
	if nm.(Model).mode != modeNormal {
		t.Fatal("esc did not close help")
	}
}
