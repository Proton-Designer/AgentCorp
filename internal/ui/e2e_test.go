package ui

import (
	"strings"
	"testing"

	"github.com/aymanmohammed/crew/internal/store"
)

// End-to-end through the real stack: SQLite -> store -> tree assembly ->
// layout -> render. Unit tests on each layer can all pass while the seams
// between them are broken; this is the test that catches that.
func TestEndToEndFromRealDB(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/e2e.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rows := []store.Node{
		{NodeID: "1", Name: "ceo", Role: "lead", ParentID: "", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:01Z"},
		{NodeID: "2", Name: "lead-be", Role: "backend", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:02Z"},
		{NodeID: "3", Name: "lead-fe", Role: "frontend", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:03Z"},
		{NodeID: "4", Name: "backend-dev", Role: "dev", ParentID: "2", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:04Z"},
		{NodeID: "5", Name: "db-dev", Role: "dev", ParentID: "2", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:05Z"},
		{NodeID: "6", Name: "ui-dev", Role: "dev", ParentID: "3", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:06Z"},
		{NodeID: "7", Name: "reviewer", Role: "review", ParentID: "3", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2026-07-16T00:00:07Z"},
	}
	for _, r := range rows {
		if err := s.InsertNode(r); err != nil {
			t.Fatalf("insert %s: %v", r.NodeID, err)
		}
	}

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 7 {
		t.Fatalf("read %d nodes, want 7", len(nodes))
	}

	m := New(nodes)
	if m.root == nil {
		t.Fatal("BuildTree returned nil root from a 7-node db")
	}
	if len(m.flat) != 7 {
		t.Fatalf("flattened %d nodes, want 7 — tree assembly dropped rows", len(m.flat))
	}

	out := Render(m.root, 80)
	for _, name := range []string{"ceo", "lead-be", "lead-fe", "backend-dev", "db-dev", "ui-dev", "reviewer"} {
		if !strings.Contains(out, name) {
			t.Fatalf("render is missing %q", name)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if w := len([]rune(line)); w > 80 {
			t.Fatalf("line %d is %d wide at the 80-col exit criterion", i, w)
		}
	}
}

// A tombstoned node must still render: §9 tombstones precisely so the
// dim=dead glyph has a row to draw, and so children keep a valid parent.
func TestDeadNodeStillRenders(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/dead.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, r := range []store.Node{
		{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		{NodeID: "2", Name: "doomed", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
		{NodeID: "3", Name: "orphan-risk", Role: "dev", ParentID: "2", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "3"},
	} {
		if err := s.InsertNode(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Tombstone("2", "2026-07-16T01:00:00Z"); err != nil {
		t.Fatal(err)
	}

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	out := Render(BuildTree(nodes), 80)
	if !strings.Contains(out, "doomed") {
		t.Fatal("tombstoned node vanished from the render; dim=dead has nothing to draw")
	}
	if !strings.Contains(out, "orphan-risk") {
		t.Fatal("child of a tombstoned node vanished — death orphaned it")
	}
}

// Child order must be deterministic. Map iteration order would make the chart
// jump between identical renders, which reads as a rendering bug.
func TestBuildTreeIsDeterministic(t *testing.T) {
	nodes := []store.Node{
		{NodeID: "1", Name: "root", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		{NodeID: "2", Name: "a", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
		{NodeID: "3", Name: "b", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "3"},
		{NodeID: "4", Name: "c", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "4"},
	}
	first := Render(BuildTree(nodes), 80)
	for i := 0; i < 20; i++ {
		if got := Render(BuildTree(nodes), 80); got != first {
			t.Fatalf("render %d differs from the first — child order is nondeterministic", i)
		}
	}
}
