package ui

import (
	"errors"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/sync"
)

func liveModel(t *testing.T) (Model, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/live.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	rows := []store.Node{
		{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		{NodeID: "2", Name: "worker", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
	}
	for _, r := range rows {
		if err := s.InsertNode(r); err != nil {
			t.Fatal(err)
		}
	}
	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	return NewLive(s, nodes), s
}

// A failed poll must NOT be rendered as if it were current. We keep the last
// known-good tree and mark it stale. Silently redrawing a stale tree as
// current is how an operator ends up trusting a chart that stopped being true
// minutes ago — the UI-layer twin of sync's "unknown is not empty" rule.
func TestFailedTickMarksStaleAndKeepsTree(t *testing.T) {
	m, _ := liveModel(t)
	before := Render(m.root, 80)

	m.applyTick(sync.TickMsg{Err: errors.New("tmux unreachable")})

	if !m.live.stale {
		t.Fatal("failed tick did not mark the view stale — the UI would present unknown state as current")
	}
	if m.root == nil {
		t.Fatal("failed tick cleared the tree; a poll failure is not evidence the org is empty")
	}
	if got := Render(m.root, 80); got != before {
		t.Fatal("failed tick mutated the rendered tree")
	}
}

// A successful tick clears staleness.
func TestSuccessfulTickClearsStale(t *testing.T) {
	m, _ := liveModel(t)
	m.applyTick(sync.TickMsg{Err: errors.New("boom")})
	if !m.live.stale {
		t.Fatal("precondition: expected stale")
	}
	m.applyTick(sync.TickMsg{})
	if m.live.stale {
		t.Fatal("successful tick did not clear stale")
	}
	if m.live.lastErr != nil {
		t.Fatalf("successful tick left lastErr = %v", m.live.lastErr)
	}
}

// Folds must survive a rebuild. The tick fires every second; if folds sprang
// open on each one, the UI would be unusable.
func TestRebuildPreservesCollapse(t *testing.T) {
	m, s := liveModel(t)
	m.flat[0].Collapsed = true
	m.reflatten()

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	m.rebuild(nodes)

	if m.root == nil || !m.root.Collapsed {
		t.Fatal("rebuild lost the collapsed state — folds would spring open every tick")
	}
}

// The cursor must survive a rebuild, by identity not index. Index-based
// restoration silently selects a different agent when the org changes shape.
func TestRebuildPreservesSelectionByIdentity(t *testing.T) {
	m, s := liveModel(t)
	m.cursor = 1
	want := m.selected().ID

	// A new node arrives ahead of the selection in creation order.
	if err := s.InsertNode(store.Node{
		NodeID: "0", Name: "aaa-new", Role: "dev", ParentID: "1",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "0",
	}); err != nil {
		t.Fatal(err)
	}
	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	m.rebuild(nodes)

	if got := m.selected(); got == nil || got.ID != want {
		gotID := "<nil>"
		if got != nil {
			gotID = got.ID
		}
		t.Fatalf("selection is now %q, want %q — the cursor jumped to a different agent", gotID, want)
	}
}

// A node vanishing under the cursor must not panic or leave a dangling index.
func TestRebuildHandlesSelectedNodeDisappearing(t *testing.T) {
	m, s := liveModel(t)
	m.cursor = 1

	m.rebuild([]store.Node{
		{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	})
	if m.cursor >= len(m.flat) {
		t.Fatalf("cursor %d out of range after the selected node vanished (len %d)", m.cursor, len(m.flat))
	}
	if m.selected() == nil {
		t.Fatal("no selection after rebuild")
	}
	_ = s
}

// A static model must never poll — tests and snapshot renders shouldn't touch
// the substrate.
func TestStaticModelHasNoTickCmd(t *testing.T) {
	m := New([]store.Node{
		{NodeID: "1", Name: "solo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	})
	if m.Init() != nil {
		t.Fatal("static model returned a tick command; it would poll the broker")
	}
	if m.tickCmd() != nil {
		t.Fatal("static model produced a tickCmd")
	}
}
