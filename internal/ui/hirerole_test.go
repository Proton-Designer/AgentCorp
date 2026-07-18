package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// After a name is entered, the hire advances to a role picker listing the
// default plus the seeded role archetypes.
func TestHireRolePickerOpens(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp",
			SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m.pendingHireName = "newbie"
	cmd := m.openHireRole()

	if m.mode != modeHireRole {
		t.Fatal("role picker did not open (default roles should be seeded)")
	}
	if cmd != nil {
		t.Fatal("should not hire directly when roles exist")
	}
	v := m.View()
	for _, want := range []string{"role for \"newbie\"", "default", "researcher", "engineer"} {
		if !strings.Contains(v, want) {
			t.Fatalf("picker missing %q:\n%s", want, v)
		}
	}
}

// The picker cursor moves over [default, ...roles] and clamps at the ends.
func TestHireRolePickerNavigation(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp",
			SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m.pendingHireName = "newbie"
	m.openHireRole()
	n := len(m.roles)

	// Down past every role must clamp at the last one, never overflow.
	for i := 0; i < n+5; i++ {
		nm, _, _ := m.handleModalKey("down")
		m = nm
	}
	if m.roleCursor != n {
		t.Fatalf("roleCursor = %d after over-scrolling, want %d (clamped)", m.roleCursor, n)
	}
	// esc closes without hiring.
	nm, _, _ := m.handleModalKey("esc")
	if nm.mode != modeNormal {
		t.Fatal("esc did not close the role picker")
	}
}
