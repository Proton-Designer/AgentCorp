package vitals

import (
	"testing"
	"time"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/store"
)

func vnode(id, peerID, parentID, state, createdAt string) store.Node {
	return store.Node{
		NodeID: id, PeerID: peerID, ParentID: parentID,
		Name: id, Role: "dev", Workdir: "/tmp", SpawnMode: "tmux-window",
		State: state, CreatedAt: createdAt,
	}
}

func vpeer(id, tty string) broker.Peer {
	return broker.Peer{ID: id, PID: 1, CWD: "/tmp", TTY: tty, RegisteredAt: "t", LastSeen: "t"}
}

var fixedNow = mustParse("2026-07-16T12:00:00Z")

func mustParse(s string) time.Time {
	t, ok := parseTimestamp(s)
	if !ok {
		panic("bad fixture timestamp: " + s)
	}
	return t
}

func TestVitalsEmptyInputsProduceZeroSummary(t *testing.T) {
	got := Vitals(nil, nil, fixedNow)
	want := Summary{}
	if got != want {
		t.Fatalf("Vitals(nil, nil) = %+v, want %+v", got, want)
	}
}

func TestVitalsCountsAliveForBoundNodeWithLivePeer(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peers := []broker.Peer{vpeer("p1", "ttys000")}

	got := Vitals(nodes, peers, fixedNow)
	if got.Alive != 1 {
		t.Fatalf("Alive = %d, want 1", got.Alive)
	}
	if got.Dead != 0 {
		t.Fatalf("Dead = %d, want 0", got.Dead)
	}
}

func TestVitalsCountsTombstonedNodeAsDead(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "dead", "2026-07-16T00:00:00Z")}
	got := Vitals(nodes, nil, fixedNow)
	if got.Dead != 1 {
		t.Fatalf("Dead = %d, want 1", got.Dead)
	}
	if got.Alive != 0 {
		t.Fatalf("Alive = %d, want 0", got.Alive)
	}
}

// A node still recorded as state='alive' whose peer has vanished from the
// broker (not yet reconciled to 'dead' in the DB) must count as Dead in a
// live snapshot — trusting the stale state field would show a peer as alive
// after its process is gone.
func TestVitalsCountsBoundButPeerGoneAsDead(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	got := Vitals(nodes, nil, fixedNow) // no peers: p1 is gone
	if got.Dead != 1 {
		t.Fatalf("Dead = %d, want 1 (stale-alive node whose peer vanished)", got.Dead)
	}
	if got.Alive != 0 {
		t.Fatalf("Alive = %d, want 0", got.Alive)
	}
}

func TestVitalsDoesNotCountPendingOrFailedNodes(t *testing.T) {
	nodes := []store.Node{
		vnode("pending1", "", "", "pending", "2026-07-16T00:00:00Z"),
		vnode("failed1", "", "", "failed", "2026-07-16T00:00:00Z"),
	}
	got := Vitals(nodes, nil, fixedNow)
	if got.Alive != 0 || got.Dead != 0 || got.Unmanaged != 0 {
		t.Fatalf("Summary = %+v, want all-zero (pending/failed nodes counted in no bucket)", got)
	}
}

func TestVitalsUnmanagedExcludesPeerClaimedByPendingBind(t *testing.T) {
	nodes := []store.Node{vnode("n1", "", "", "pending", "2026-07-16T00:00:00Z")}
	nodes[0].BindTTY = "/dev/ttys024"
	peers := []broker.Peer{vpeer("p1", "ttys024")} // matches n1's bind_tty after normalization

	got := Vitals(nodes, peers, fixedNow)
	if got.Unmanaged != 0 {
		t.Fatalf("Unmanaged = %d, want 0 (p1 is claimed by a pending bind, not free-floating)", got.Unmanaged)
	}
}

func TestVitalsUnmanagedCountsTrulyFreePeers(t *testing.T) {
	peers := []broker.Peer{vpeer("p1", "ttys000"), vpeer("p2", "ttys001")}
	got := Vitals(nil, peers, fixedNow)
	if got.Unmanaged != 2 {
		t.Fatalf("Unmanaged = %d, want 2", got.Unmanaged)
	}
}

func TestVitalsUptimeIsSinceEarliestNodeCreatedAt(t *testing.T) {
	nodes := []store.Node{
		vnode("n1", "", "", "pending", "2026-07-16T10:00:00Z"),
		vnode("n2", "", "", "pending", "2026-07-16T08:00:00Z"), // earlier
		vnode("n3", "", "", "pending", "2026-07-16T11:00:00Z"),
	}
	got := Vitals(nodes, nil, fixedNow) // fixedNow = 2026-07-16T12:00:00Z
	want := 4 * time.Hour               // 12:00 - 08:00
	if got.Uptime != want {
		t.Fatalf("Uptime = %v, want %v", got.Uptime, want)
	}
}

func TestVitalsUptimeZeroWithNoNodes(t *testing.T) {
	got := Vitals(nil, nil, fixedNow)
	if got.Uptime != 0 {
		t.Fatalf("Uptime = %v, want 0", got.Uptime)
	}
}

func TestVitalsUptimeSkipsUnparseableCreatedAt(t *testing.T) {
	nodes := []store.Node{
		vnode("bad", "", "", "pending", "not-a-timestamp"),
		vnode("good", "", "", "pending", "2026-07-16T10:00:00Z"),
	}
	got := Vitals(nodes, nil, fixedNow)
	want := 2 * time.Hour // 12:00 - 10:00, "bad" ignored rather than crashing or winning as earliest
	if got.Uptime != want {
		t.Fatalf("Uptime = %v, want %v", got.Uptime, want)
	}
}

// Canary: Working/Idle/Blocked must stay at zero until a real signal exists.
// If this test breaks because someone populated them, that's the signal to
// update this test deliberately — not evidence the change is wrong, just a
// tripwire so it can't happen silently via an unreviewed heuristic.
func TestVitalsWorkingIdleBlockedAreNotComputed(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peers := []broker.Peer{vpeer("p1", "ttys000")}
	got := Vitals(nodes, peers, fixedNow)
	if got.Working != 0 || got.Idle != 0 || got.Blocked != 0 {
		t.Fatalf("Working/Idle/Blocked = %d/%d/%d, want 0/0/0 (no reliable signal exists yet)",
			got.Working, got.Idle, got.Blocked)
	}
}
