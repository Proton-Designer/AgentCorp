package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'R' opens a rename field pre-filled with the current name; submitting renames.
func TestRenameFlow(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "oldname", Role: "lead", Workdir: "/t",
			SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m = send(m, "R")
	if m.mode != modeRename {
		t.Fatalf("R did not open rename (mode=%v)", m.mode)
	}
	if m.renameInput.value != "oldname" {
		t.Fatalf("rename field not pre-filled, got %q", m.renameInput.value)
	}
	// Clear and type a new name.
	m.renameInput.value = "newname"
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd != nil {
		cmd()
	}
	nodes, _ := s.ListNodes()
	if nodes[0].Name != "newname" {
		t.Fatalf("name = %q after rename, want newname", nodes[0].Name)
	}
}

// Renaming to a name already used by another live node is rejected.
func TestRenameRejectsCollision(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "alice", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "bob", Role: "dev", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
	)
	m = send(m, "down") // select bob
	m = send(m, "R")
	m.renameInput.value = "alice" // collide with node 1
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	var text string
	if cmd != nil {
		if msg, ok := cmd().(actionResultMsg); ok {
			text = msg.text
		}
	}
	if !strings.Contains(text, "already taken") {
		t.Fatalf("collision not rejected, got %q", text)
	}
	nodes, _ := s.ListNodes()
	for _, n := range nodes {
		if n.NodeID == "2" && n.Name != "bob" {
			t.Fatalf("bob was renamed to a taken name: %q", n.Name)
		}
	}
}
