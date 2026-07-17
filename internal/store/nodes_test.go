package store

import (
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mkNode(id, parent string) Node {
	return Node{
		NodeID: id, Name: id, Role: "dev", ParentID: parent,
		Workdir: "/tmp", SpawnMode: "tmux-window",
		State: "pending", CreatedAt: "2026-07-16T00:00:00Z",
	}
}

// The UNIQUE(peer_id) constraint: a binding race must fail loudly rather than
// silently double-bind two nodes to one live agent (spec §9).
func TestBindPeerRejectsDoubleBind(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("n1", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertNode(mkNode("n2", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.BindPeer("n1", "abc123"); err != nil {
		t.Fatalf("first bind should succeed: %v", err)
	}
	err := s.BindPeer("n2", "abc123")
	if err == nil {
		t.Fatal("second bind to the same peer_id must fail, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("want a UNIQUE violation, got: %v", err)
	}
}

// Multiple unbound nodes must coexist. SQLite exempts NULL from UNIQUE, so
// this passes — but only if we store NULL rather than "".
func TestManyPendingNodesCoexist(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"a", "b", "c"} {
		if err := s.InsertNode(mkNode(id, "")); err != nil {
			t.Fatalf("pending node %s: %v", id, err)
		}
	}
}

// Tombstone, never hard-delete: a dead parent's row survives so children keep
// a valid parent_id and §5's dim=dead glyph has a row to render.
func TestTombstoneRetainsRowAndChildren(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("parent", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertNode(mkNode("child", "parent")); err != nil {
		t.Fatal(err)
	}
	if err := s.Tombstone("parent", "2026-07-16T01:00:00Z"); err != nil {
		t.Fatal(err)
	}

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (tombstone must retain the row)", len(nodes))
	}
	for _, n := range nodes {
		switch n.NodeID {
		case "parent":
			if n.State != "dead" || n.DiedAt == "" {
				t.Fatalf("parent: state=%q died_at=%q, want dead + timestamp", n.State, n.DiedAt)
			}
		case "child":
			if n.ParentID != "parent" {
				t.Fatalf("child.ParentID = %q, want \"parent\" (death must not orphan)", n.ParentID)
			}
		}
	}
}

// DeleteNode removes the row entirely — the operator-ordered death path, as
// opposed to Tombstone's observed-death path that keeps the row.
func TestDeleteNodeRemovesRow(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("keep", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertNode(mkNode("gone", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNode("gone"); err != nil {
		t.Fatal(err)
	}
	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "keep" {
		t.Fatalf("after delete, got %+v; want only \"keep\"", nodes)
	}
}

// FK enforcement is live (Task 1's pragma actually bites).
func TestInsertRejectsUnknownParent(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("orphan", "does-not-exist")); err == nil {
		t.Fatal("insert with unknown parent_id must fail; FK enforcement is off")
	}
}

// pending -> alive is the only legal transition into alive. Binding a
// tombstoned node must fail loudly, not resurrect it — by the time a node is
// dead, reparenting (§6.3) may already have moved its children away, so a
// resurrected zombie parent would render a chart that lies.
func TestBindPeerRejectsDeadNode(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("n1", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.Tombstone("n1", "2026-07-16T01:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.BindPeer("n1", "abc123"); err == nil {
		t.Fatal("binding a tombstoned node must fail, got nil — a dead node was resurrected")
	}
	// And it must still be dead.
	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].State != "dead" {
		t.Fatalf("state = %q after rejected bind, want dead", nodes[0].State)
	}
}
