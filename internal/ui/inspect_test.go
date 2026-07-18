package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'i' opens the inspector for the selected agent and shows its real details.
func TestInspectShowsAgentDetail(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp/work",
			SpawnMode: "tmux-window", SpawnRef: "%3", State: "alive", PeerID: "boss",
			CreatedAt: "2026-07-18T06:00:00Z"},
	)
	m.live.peers = []broker.Peer{{ID: "boss", CWD: "/tmp/work", TTY: "ttys9", Summary: "leading the org"}}
	m.live.msgs = []broker.Message{
		{FromID: "boss", ToID: "x", Text: "status?"},
		{FromID: "y", ToID: "boss", Text: "done"},
	}

	m = send(m, "i")
	if m.mode != modeInspect {
		t.Fatalf("'i' did not open the inspector (mode=%v)", m.mode)
	}
	v := m.View()
	for _, want := range []string{"ceo", "lead", "boss", "ttys9", "leading the org", "1 sent · 1 recv"} {
		if !strings.Contains(v, want) {
			t.Fatalf("inspector missing %q:\n%s", want, v)
		}
	}
	// esc closes it.
	nm, _ := m.Update(key("esc"))
	if nm.(Model).mode != modeNormal {
		t.Fatal("esc did not close the inspector")
	}
}

// An adopted node's role is labeled as a guess — we don't own its prompt.
func TestInspectMarksAdoptedRoleAsGuess(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "wild", Role: "adopted", Workdir: "/x",
			SpawnMode: "adopted", State: "alive", PeerID: "p1", CreatedAt: "2026-07-18T06:00:00Z"},
	)
	m.live.peers = []broker.Peer{{ID: "p1", CWD: "/x"}}
	m = send(m, "i")
	if !strings.Contains(m.View(), "guess") {
		t.Fatalf("adopted node's role not marked as a guess:\n%s", m.View())
	}
}

// Inspecting while navigating: ↑↓ move the selection with the panel still open.
func TestInspectNavigatesWhileOpen(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "worker", Role: "dev", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
	)
	m = send(m, "i")
	before := m.cursor
	m = send(m, "down")
	if m.mode != modeInspect {
		t.Fatal("navigating closed the inspector; it should stay open")
	}
	if m.cursor == before {
		t.Fatal("↓ did not move the selection while inspecting")
	}
}
