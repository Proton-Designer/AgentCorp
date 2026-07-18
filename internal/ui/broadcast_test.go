package ui

import (
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// A broadcast reaches only the live, bound descendants — never the root itself,
// never unbound or dead nodes (there's no peer to deliver to).
func TestBroadcastTargetsAreLiveBoundDescendants(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "boss", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "mgr", Role: "lead", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "p2", CreatedAt: "2"},
		store.Node{NodeID: "3", Name: "forming", Role: "dev", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "pending", CreatedAt: "3"},
		store.Node{NodeID: "4", Name: "gone", Role: "dev", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "dead", PeerID: "p4", CreatedAt: "4"},
		store.Node{NodeID: "5", Name: "worker", Role: "dev", ParentID: "2", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "p5", CreatedAt: "5"},
	)

	targets, err := m.broadcastTargets("ceo")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range targets {
		got[n.PeerID] = true
	}
	if !got["p2"] || !got["p5"] {
		t.Fatalf("live bound descendants missing: %v", got)
	}
	if got["boss"] {
		t.Fatal("broadcast included the root itself — you message your team, not yourself")
	}
	if got["p4"] {
		t.Fatal("broadcast included a dead node")
	}
	if len(got) != 2 {
		t.Fatalf("targets = %v, want exactly {p2, p5}", got)
	}
}

// 'b' on a node with no reachable team flashes instead of opening an empty
// compose box.
func TestBroadcastRefusesEmptyTeam(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "solo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "boss", CreatedAt: "1"},
	)
	m = send(m, "b")
	if m.mode == modeBroadcast {
		t.Fatal("broadcast opened with no reachable team")
	}
}
