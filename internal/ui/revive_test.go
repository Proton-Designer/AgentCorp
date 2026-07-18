package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'z' on a live node says there's nothing to revive.
func TestReviveRejectsLiveNode(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "p", CreatedAt: "1"},
	)
	cmd := m.submitRevive()
	if cmd == nil {
		t.Fatal("expected a flash")
	}
	if msg, ok := cmd().(actionResultMsg); !ok || !strings.Contains(msg.text, "isn't dead") {
		t.Fatalf("expected 'isn't dead' guidance, got %+v", cmd())
	}
}

// 'z' on a dead node whose session is gone points to delete/adopt.
func TestReviveDeadNoSessionGuidesToDeleteOrAdopt(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ghost", Role: "agent", Workdir: "/t", SpawnMode: "tmux-window", State: "dead", CreatedAt: "1", DiedAt: "2026-07-18T06:00:00Z"},
	)
	cmd := m.submitRevive()
	msg, ok := cmd().(actionResultMsg)
	if !ok {
		t.Fatalf("no result: %+v", cmd())
	}
	if !strings.Contains(msg.text, "delete") || !strings.Contains(msg.text, "adopt") {
		t.Fatalf("expected delete/adopt guidance, got %q", msg.text)
	}
}

// shift-Z with no dead agents says so.
func TestReviveAllNoDead(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "p", CreatedAt: "1"},
	)
	msg, ok := m.submitReviveAll()().(actionResultMsg)
	if !ok || !strings.Contains(msg.text, "no dead agents") {
		t.Fatalf("expected 'no dead agents', got %+v", msg)
	}
}

// shift-Z when all dead agents lack a resumable session points to fire/adopt.
func TestReviveAllNoneRevivable(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ghost1", Role: "agent", Workdir: "/t", SpawnMode: "tmux-window", State: "dead", CreatedAt: "1", DiedAt: "2026-07-18T06:00:00Z"},
		store.Node{NodeID: "2", Name: "ghost2", Role: "agent", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "dead", CreatedAt: "2", DiedAt: "2026-07-18T06:00:00Z"},
	)
	msg, ok := m.submitReviveAll()().(actionResultMsg)
	if !ok || !strings.Contains(msg.text, "no revivable") || !strings.Contains(msg.text, "2 have no resumable session") {
		t.Fatalf("expected guidance naming 2 unrevivable, got %+v", msg)
	}
}
