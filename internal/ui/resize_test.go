package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Resizing must issue a clear so a bare (non-tmux) terminal doesn't stack the
// old frame above the new one after a minimize/restore.
func TestResizeClearsScreen(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd == nil {
		t.Fatal("resize should issue a clear-screen command")
	}
}
