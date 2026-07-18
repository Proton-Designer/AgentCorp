package ui

import (
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'r' opens the move picker and enter reparents the selected node under the
// chosen target; a descendant is never offered as a target.
func TestMoveFlow(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "mgr", Role: "lead", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
		store.Node{NodeID: "3", Name: "worker", Role: "dev", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "3"},
	)
	m = send(m, "down") // select mgr
	m = send(m, "r")
	if m.mode != modeMove {
		t.Fatalf("r did not open move (mode=%v)", m.mode)
	}
	// Targets should include worker (3) but never mgr itself.
	found := false
	for _, tg := range m.moveTargets {
		if tg.NodeID == "2" {
			t.Fatal("move offered the mover itself as a target")
		}
		if tg.NodeID == "3" {
			found = true
		}
	}
	if !found {
		t.Fatal("worker not offered as a move target")
	}
	// Move mgr to root (cursor 0) and confirm.
	m.moveCursor = 0
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd != nil {
		cmd()
	}
	nodes, _ := s.ListNodes()
	for _, n := range nodes {
		if n.NodeID == "2" && n.ParentID != "" {
			t.Fatalf("mgr not moved to root, parent=%q", n.ParentID)
		}
	}
}
