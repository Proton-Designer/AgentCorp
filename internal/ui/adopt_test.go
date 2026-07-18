package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'a' opens the adopt picker only when there are unmanaged peers; enter adopts
// the selected one as a live, bound node under the selected tree node.
func TestAdoptFlow(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp",
			SpawnMode: "tmux-window", State: "alive", PeerID: "boss", CreatedAt: "1"},
	)
	// Make one in-company peer look unmanaged, and keep it in the live set so the
	// still-alive re-check passes.
	m.live.peers = []broker.Peer{
		{ID: "boss", CWD: "/tmp"},
		{ID: "wild1", CWD: "/tmp/project", Summary: "refactoring the parser"},
	}
	m.live.unmanaged = []broker.Peer{{ID: "wild1", CWD: "/tmp/project", Summary: "refactoring the parser"}}

	m = send(m, "a")
	if m.mode != modeAdopt {
		t.Fatalf("'a' did not open the adopt picker (mode=%v)", m.mode)
	}
	if !strings.Contains(m.View(), "refactoring the parser") {
		t.Fatalf("adopt picker doesn't show the peer summary:\n%s", m.View())
	}

	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd != nil {
		cmd()
	}
	if m.mode != modeNormal {
		t.Fatal("adopt picker didn't close after enter")
	}

	// A new alive, bound, adopted node must exist for peer wild1, under ceo.
	nodes, _ := s.ListNodes()
	var got *store.Node
	for i := range nodes {
		if nodes[i].PeerID == "wild1" {
			got = &nodes[i]
		}
	}
	if got == nil {
		t.Fatal("no node was created for the adopted peer")
	}
	if got.State != "alive" {
		t.Fatalf("adopted node state = %q, want alive", got.State)
	}
	if got.SpawnMode != "adopted" || got.SpawnRef != "" {
		t.Fatalf("adopted node should carry spawn_mode=adopted and no spawn_ref, got mode=%q ref=%q", got.SpawnMode, got.SpawnRef)
	}
	if got.ParentID != "1" {
		t.Fatalf("adopted under %q, want ceo (\"1\")", got.ParentID)
	}
}

// Adopting a peer that vanished between selection and confirm must fail loudly,
// never insert a corpse.
func TestAdoptRefusesDeadPeer(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp",
			SpawnMode: "tmux-window", State: "alive", PeerID: "boss", CreatedAt: "1"},
	)
	// The picker lists wild1, but it's NOT in the live peer set anymore.
	m.live.peers = []broker.Peer{{ID: "boss", CWD: "/tmp"}}
	m.live.unmanaged = []broker.Peer{{ID: "wild1", CWD: "/tmp/project"}}

	m = send(m, "a")
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd != nil {
		if msg, ok := cmd().(actionResultMsg); ok && !strings.Contains(msg.text, "gone") {
			t.Fatalf("expected a 'gone' message, got %q", msg.text)
		}
	}
	nodes, _ := s.ListNodes()
	for _, n := range nodes {
		if n.PeerID == "wild1" {
			t.Fatal("a dead peer was adopted anyway — corpse inserted")
		}
	}
}

// 'a' with no unmanaged peers just flashes, never opens an empty picker.
func TestAdoptNoCandidates(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp",
			SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m.live.unmanaged = nil
	m = send(m, "a")
	if m.mode == modeAdopt {
		t.Fatal("adopt picker opened with no candidates")
	}
}
