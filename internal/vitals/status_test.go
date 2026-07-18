package vitals

import (
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

func TestNodeStatusDead(t *testing.T) {
	n := vnode("n1", "p1", "", "dead", "2026-07-16T00:00:00Z")
	if got := NodeStatus(n, nil, nil, fixedNow, testWindow); got != StatusDead {
		t.Fatalf("NodeStatus = %q, want %q", got, StatusDead)
	}
}

func TestNodeStatusBoundButPeerGoneIsDead(t *testing.T) {
	n := vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")
	if got := NodeStatus(n, nil, nil, fixedNow, testWindow); got != StatusDead {
		t.Fatalf("NodeStatus = %q, want %q (bound peer absent from broker)", got, StatusDead)
	}
}

func TestNodeStatusPendingHasNoPeer(t *testing.T) {
	n := vnode("n1", "", "", "pending", "2026-07-16T00:00:00Z")
	if got := NodeStatus(n, nil, nil, fixedNow, testWindow); got != StatusPending {
		t.Fatalf("NodeStatus = %q, want %q", got, StatusPending)
	}
}

func TestNodeStatusFailedHasNoPeer(t *testing.T) {
	n := vnode("n1", "", "", "failed", "2026-07-16T00:00:00Z")
	if got := NodeStatus(n, nil, nil, fixedNow, testWindow); got != StatusPending {
		t.Fatalf("NodeStatus = %q, want %q", got, StatusPending)
	}
}

func TestNodeStatusActiveWhenRecentSender(t *testing.T) {
	n := vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")
	peers := []broker.Peer{vpeer("p1", "ttys000")}
	msgs := []broker.Message{vmsg("p1", "2026-07-16T11:59:45Z")} // 15s before fixedNow

	if got := NodeStatus(n, peers, msgs, fixedNow, testWindow); got != StatusActive {
		t.Fatalf("NodeStatus = %q, want %q", got, StatusActive)
	}
}

func TestNodeStatusQuietWhenStaleSender(t *testing.T) {
	n := vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")
	peers := []broker.Peer{vpeer("p1", "ttys000")}
	msgs := []broker.Message{vmsg("p1", "2026-07-16T10:00:00Z")} // 2h before fixedNow

	if got := NodeStatus(n, peers, msgs, fixedNow, testWindow); got != StatusQuiet {
		t.Fatalf("NodeStatus = %q, want %q", got, StatusQuiet)
	}
}

func TestNodeStatusQuietWhenNeverSpoke(t *testing.T) {
	n := vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")
	peers := []broker.Peer{vpeer("p1", "ttys000")}

	if got := NodeStatus(n, peers, nil, fixedNow, testWindow); got != StatusQuiet {
		t.Fatalf("NodeStatus = %q, want %q", got, StatusQuiet)
	}
}

// Another peer's recent message must never leak into this node's status.
func TestNodeStatusIgnoresOtherPeersActivity(t *testing.T) {
	n := vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")
	peers := []broker.Peer{vpeer("p1", "ttys000"), vpeer("p2", "ttys001")}
	msgs := []broker.Message{vmsg("p2", "2026-07-16T11:59:59Z")} // p2 active, not p1

	if got := NodeStatus(n, peers, msgs, fixedNow, testWindow); got != StatusQuiet {
		t.Fatalf("NodeStatus = %q, want %q (p2's activity must not count for p1)", got, StatusQuiet)
	}
}

// NodeStatus must never return StatusUnreachable — that value is reserved
// for an impure caller that layers §5.1's ps-based heuristic on top; it
// requires OS process-table access this pure function cannot perform.
func TestNodeStatusNeverReturnsUnreachable(t *testing.T) {
	states := []string{"pending", "alive", "dead", "failed"}
	for _, state := range states {
		n := vnode("n1", "p1", "", state, "2026-07-16T00:00:00Z")
		if got := NodeStatus(n, nil, nil, fixedNow, testWindow); got == StatusUnreachable {
			t.Fatalf("NodeStatus(state=%q) = %q, must never be StatusUnreachable from this pure function", state, got)
		}
	}
}

// A freshly-hired node whose peer hasn't shown up in the broker snapshot yet
// must NOT flash 'dead' — it's registration lag, shown as still-settling. An
// established node with an absent peer is still dead promptly.
func TestNodeStatusFreshHireDoesNotFlashDead(t *testing.T) {
	// fixedNow = 2026-07-16T12:00:00Z; grace = 20s.
	fresh := vnode("n1", "p1", "", "alive", "2026-07-16T11:59:55Z") // 5s old, peer absent
	if got := NodeStatus(fresh, nil, nil, fixedNow, testWindow); got != StatusPending {
		t.Fatalf("fresh bound node with lagging peer = %q, want pending (no dead flash)", got)
	}
	old := vnode("n2", "p2", "", "alive", "2026-07-16T11:00:00Z") // an hour old, peer absent
	if got := NodeStatus(old, nil, nil, fixedNow, testWindow); got != StatusDead {
		t.Fatalf("established bound node with absent peer = %q, want dead", got)
	}
	// Peer present → alive regardless of age.
	live := []broker.Peer{{ID: "p1"}}
	if got := NodeStatus(fresh, live, nil, fixedNow, testWindow); got != StatusQuiet {
		t.Fatalf("bound node with live peer = %q, want quiet", got)
	}
}
