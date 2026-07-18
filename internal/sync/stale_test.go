package sync

import (
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func TestStalePendingFailsOldUnboundOnly(t *testing.T) {
	now := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	old := now.Add(-10 * time.Minute).Format(time.RFC3339)
	fresh := now.Add(-10 * time.Second).Format(time.RFC3339)

	nodes := []store.Node{
		{NodeID: "old-pending", State: "pending", CreatedAt: old},                     // -> fail
		{NodeID: "fresh-pending", State: "pending", CreatedAt: fresh},                 // still forming
		{NodeID: "old-bound", State: "alive", PeerID: "p", CreatedAt: old},            // bound, never fail
		{NodeID: "old-pending-bound", State: "pending", PeerID: "p2", CreatedAt: old}, // has peer, not unbound
		{NodeID: "unparseable", State: "pending", CreatedAt: "1"},                     // can't prove stale
	}
	got := StalePending(nodes, now, PendingGrace)
	if len(got) != 1 || got[0] != "old-pending" {
		t.Fatalf("StalePending = %v, want [old-pending]", got)
	}
}

func TestStalePendingEmptyWhenAllFresh(t *testing.T) {
	now := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	nodes := []store.Node{
		{NodeID: "a", State: "pending", CreatedAt: now.Format(time.RFC3339)},
	}
	if got := StalePending(nodes, now, PendingGrace); len(got) != 0 {
		t.Fatalf("StalePending = %v, want empty", got)
	}
}
