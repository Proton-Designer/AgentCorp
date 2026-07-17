package broker

import (
	"testing"

	"github.com/aymanmohammed/crew/internal/store"
)

func node(id, peerID, bindTTY, parentID, state string) store.Node {
	return store.Node{
		NodeID: id, PeerID: peerID, BindTTY: bindTTY, ParentID: parentID,
		Name: id, Role: "dev", Workdir: "/tmp", SpawnMode: "tmux-window",
		State: state, CreatedAt: "2026-07-16T00:00:00Z",
	}
}

func peer(id, tty string) Peer {
	return Peer{ID: id, PID: 1, CWD: "/tmp", TTY: tty, RegisteredAt: "t", LastSeen: "t"}
}

// One scenario exercising all four buckets together, since Reconcile's whole
// job is classifying a mixed set correctly, not any bucket in isolation.
func TestReconcile(t *testing.T) {
	nodes := []store.Node{
		node("alive1", "peer-alive", "", "", "alive"),
		node("dead1", "peer-gone", "", "", "alive"), // bound, but peer no longer in broker
		node("tombstoned1", "peer-long-gone", "", "", "dead"),
		node("pending1", "", "/dev/ttys024", "", "pending"),
		node("pending-no-match", "", "/dev/ttys999", "", "pending"),
		node("pending-no-tty", "", "", "", "pending"),
	}
	peers := []Peer{
		peer("peer-alive", "ttys000"),
		peer("peer-unmanaged", "ttys001"), // no node points at this one
		peer("peer-new", "ttys024"),       // matches pending1's tty (post-normalization)
	}

	rec := Reconcile(nodes, peers)

	if len(rec.Alive) != 1 || rec.Alive[0].NodeID != "alive1" {
		t.Fatalf("Alive = %+v, want exactly [alive1]", rec.Alive)
	}
	if len(rec.Dead) != 1 || rec.Dead[0].NodeID != "dead1" {
		t.Fatalf("Dead = %+v, want exactly [dead1] (tombstoned1 must NOT reappear, it's already dead)", rec.Dead)
	}
	if len(rec.Unmanaged) != 1 || rec.Unmanaged[0].ID != "peer-unmanaged" {
		t.Fatalf("Unmanaged = %+v, want exactly [peer-unmanaged]", rec.Unmanaged)
	}
	if len(rec.PendingBind) != 1 || rec.PendingBind[0] != (Binding{NodeID: "pending1", PeerID: "peer-new"}) {
		t.Fatalf("PendingBind = %+v, want exactly [{pending1 peer-new}]", rec.PendingBind)
	}
}

// A node already tombstoned must never be re-surfaced as newly Dead just
// because its old peer_id (correctly) isn't in the broker anymore.
func TestReconcileDoesNotReDeadAnAlreadyTombstonedNode(t *testing.T) {
	nodes := []store.Node{node("n1", "peer-long-gone", "", "", "dead")}
	rec := Reconcile(nodes, nil)

	if len(rec.Dead) != 0 {
		t.Fatalf("Dead = %+v, want empty — already-tombstoned nodes need no action", rec.Dead)
	}
	if len(rec.Alive) != 0 {
		t.Fatalf("Alive = %+v, want empty", rec.Alive)
	}
}

// PendingBind must match through NormalizeTTY, not exact string equality —
// this is the one property the whole binding mechanism depends on (spec §6.1).
func TestReconcilePendingBindNormalizesTTY(t *testing.T) {
	nodes := []store.Node{node("n1", "", "/dev/ttys024", "", "pending")}
	peers := []Peer{peer("p1", "ttys024")} // bare, as the broker actually stores it

	rec := Reconcile(nodes, peers)

	if len(rec.PendingBind) != 1 || rec.PendingBind[0].PeerID != "p1" {
		t.Fatalf("PendingBind = %+v, want a match via normalized tty comparison", rec.PendingBind)
	}
}

// A node whose peer_id was bound and is now tombstoned must not make a live
// peer that happens to reuse the concept "unmanaged" incorrectly — this
// tests the boundPeerIDs set includes dead nodes' historical bindings.
func TestReconcileUnmanagedExcludesHistoricallyBoundPeers(t *testing.T) {
	nodes := []store.Node{node("n1", "peer-x", "", "", "dead")}
	peers := []Peer{peer("peer-x", "ttys000")} // this exact id, still (hypothetically) present

	rec := Reconcile(nodes, peers)

	if len(rec.Unmanaged) != 0 {
		t.Fatalf("Unmanaged = %+v, want empty — peer-x was bound to n1, however n1's state now", rec.Unmanaged)
	}
}

// A peer matched into PendingBind must not simultaneously appear in
// Unmanaged — it's already spoken for. Caught by TestReconcile itself during
// development; kept as its own focused regression test.
func TestReconcilePendingBindPeerExcludedFromUnmanaged(t *testing.T) {
	nodes := []store.Node{node("n1", "", "/dev/ttys024", "", "pending")}
	peers := []Peer{peer("peer-new", "ttys024")}

	rec := Reconcile(nodes, peers)

	if len(rec.PendingBind) != 1 {
		t.Fatalf("PendingBind = %+v, want exactly one match (test fixture invalid)", rec.PendingBind)
	}
	if len(rec.Unmanaged) != 0 {
		t.Fatalf("Unmanaged = %+v, want empty — peer-new is claimed by the pending bind", rec.Unmanaged)
	}
}

func TestReconcileEmptyInputsProduceEmptyOutput(t *testing.T) {
	rec := Reconcile(nil, nil)
	if len(rec.Alive) != 0 || len(rec.Dead) != 0 || len(rec.Unmanaged) != 0 || len(rec.PendingBind) != 0 {
		t.Fatalf("Reconcile(nil, nil) = %+v, want all-empty", rec)
	}
}
