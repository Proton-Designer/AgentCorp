package sync

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// The one race worth pinning explicitly: a node proposed for BOTH Tombstone and
// Bind in the same apply (a pane died AND a peer appeared on its reused tty)
// must end DEAD, never corrupted-alive. The safety is apply's ordering
// (Tombstone before Bind) plus BindPeer's WHERE state='pending' guard — if that
// ordering is ever reversed or wrapped in a way that loses it, this test fails.
func TestApplyTombstoneBeatsBindOnCollision(t *testing.T) {
	st := newTickTestStore(t)
	if err := st.InsertNode(store.Node{
		NodeID: "n1", Name: "x", Role: "r", Workdir: "/tmp",
		SpawnMode: "tmux-window", SpawnRef: "%1", BindTTY: "ttys1",
		State: "pending", CreatedAt: "2026-07-18T06:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	err := apply(st, Actions{
		Tombstone: []string{"n1"},
		Bind:      []broker.Binding{{NodeID: "n1", PeerID: "p1"}},
	})
	// The bind is expected to fail loudly (node already dead) — that clean
	// rejection is the guard working, not a corruption.
	if err == nil || !strings.Contains(err.Error(), "not found or not pending") {
		t.Fatalf("expected a clean bind rejection after tombstone, got %v", err)
	}

	nodes, _ := st.ListNodes()
	for _, n := range nodes {
		if n.NodeID == "n1" {
			if n.State != "dead" {
				t.Fatalf("n1 state = %q, want dead — tombstone must win the collision", n.State)
			}
			if n.PeerID != "" {
				t.Fatalf("n1 bound to %q despite being tombstoned — corruption", n.PeerID)
			}
		}
	}
}

// apply marks a Fail'd node failed.
func TestApplyFailsStaleNode(t *testing.T) {
	st := newTickTestStore(t)
	if err := st.InsertNode(store.Node{
		NodeID: "n1", Name: "x", Role: "r", Workdir: "/tmp",
		SpawnMode: "tmux-window", State: "pending", CreatedAt: "2026-07-18T06:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := apply(st, Actions{Fail: []string{"n1"}}); err != nil {
		t.Fatal(err)
	}
	nodes, _ := st.ListNodes()
	for _, n := range nodes {
		if n.NodeID == "n1" && n.State != "failed" {
			t.Fatalf("n1 state = %q, want failed", n.State)
		}
	}
}
