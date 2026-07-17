package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func liveModelWith(t *testing.T, rows ...store.Node) (Model, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/i.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, r := range rows {
		if err := s.InsertNode(r); err != nil {
			t.Fatal(err)
		}
	}
	nodes, _ := s.ListNodes()
	m := NewLive(s, nodes)
	m.live.brokerDB = t.TempDir() + "/broker.db" // never the real one
	return m, s
}

func key(s string) tea.KeyMsg {
	if s == " " {
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func send(m Model, keys ...string) Model {
	for _, k := range keys {
		nm, _ := m.Update(key(k))
		m = nm.(Model)
	}
	return m
}

// h opens the hire modal, keystrokes build the name, esc cancels.
func TestHireModalOpensAndCancels(t *testing.T) {
	m, _ := liveModelWith(t, store.Node{NodeID: "1", Name: "ceo", Role: "lead",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"})

	m = send(m, "h")
	if m.mode != modeHire {
		t.Fatalf("h did not open the hire modal, mode = %v", m.mode)
	}
	m = send(m, "d", "e", "v")
	if m.hireInput.value != "dev" {
		t.Fatalf("typed input = %q, want \"dev\"", m.hireInput.value)
	}
	m = send(m, "esc")
	if m.mode != modeNormal {
		t.Fatal("esc did not close the modal")
	}
}

// While a modal is open, navigation keys must NOT move the cursor — they're
// text input. A 'j' typed into a name field is a letter, not a move.
func TestModalCapturesNavigationKeys(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "worker", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
	)
	cursorBefore := m.cursor
	m = send(m, "h")      // open hire
	m = send(m, "j", "k") // these must be TEXT, not movement
	if m.cursor != cursorBefore {
		t.Fatal("navigation keys moved the cursor while a modal was open")
	}
	if m.hireInput.value != "jk" {
		t.Fatalf("modal did not capture nav keys as text: %q", m.hireInput.value)
	}
}

// backspace edits the field.
func TestModalBackspace(t *testing.T) {
	m, _ := liveModelWith(t, store.Node{NodeID: "1", Name: "ceo", Role: "lead",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"})
	m = send(m, "h", "a", "b", "c")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = nm.(Model)
	if m.hireInput.value != "ab" {
		t.Fatalf("after backspace = %q, want \"ab\"", m.hireInput.value)
	}
}

// Search filters the tree without removing nodes (removing a parent orphans
// its matching children on screen).
func TestSearchFiltersWithoutRemoving(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "backend", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
	)
	m = send(m, "/", "b", "a", "c", "k")
	if m.filter != "back" {
		t.Fatalf("filter = %q, want \"back\"", m.filter)
	}
	if !m.matchesFilter(&layout.Node{ID: "backend"}) {
		t.Fatal("matching node does not match")
	}
	if m.matchesFilter(&layout.Node{ID: "ceo"}) {
		t.Fatal("non-matching node matched")
	}
}

// x on a leaf opens a fire confirm; f fires. The victim is removed from the
// chart outright — an explicit fire should leave nothing behind, not a dead
// marker that reads as "nothing happened".
func TestFireFlow(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "worker", Role: "dev", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
	)
	m = send(m, "down") // select worker
	m = send(m, "x")
	if m.mode != modeConfirm || m.confirm == nil || m.confirm.kind != confirmFire {
		t.Fatalf("x did not open a fire confirm (mode=%v)", m.mode)
	}
	// f executes; the doFire command runs synchronously enough to drive here.
	nm, cmd := m.Update(key("f"))
	m = nm.(Model)
	if m.mode != modeNormal {
		t.Fatal("fire confirm did not close after f")
	}
	if cmd != nil {
		cmd() // run the action
	}
	// worker (node 2) should be gone from the store entirely.
	nodes, _ := s.ListNodes()
	for _, n := range nodes {
		if n.NodeID == "2" {
			t.Fatalf("worker still present after fire (state=%q); an explicit fire must remove the node", n.State)
		}
	}
}

// Firing a manager reparents its reports to the grandparent, then removes the
// manager — the reports must survive, never orphaned by the removal.
func TestFireReparentsThenRemoves(t *testing.T) {
	m, s := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "mgr", Role: "lead", ParentID: "1", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "2"},
		store.Node{NodeID: "3", Name: "report", Role: "dev", ParentID: "2", Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "3"},
	)
	m = send(m, "down") // select mgr
	m = send(m, "x")
	nm, cmd := m.Update(key("f"))
	m = nm.(Model)
	if cmd != nil {
		cmd()
	}
	nodes, _ := s.ListNodes()
	var report *store.Node
	for i := range nodes {
		if nodes[i].NodeID == "2" {
			t.Fatal("manager still present after fire; it should be removed")
		}
		if nodes[i].NodeID == "3" {
			report = &nodes[i]
		}
	}
	if report == nil {
		t.Fatal("report was removed along with its manager — a fire must not orphan/lose children")
	}
	if report.ParentID != "1" {
		t.Fatalf("report reparented to %q, want the grandparent \"1\"", report.ParentID)
	}
}

// The footer advertises the real keys so an operator can discover them.
func TestFooterAdvertisesActions(t *testing.T) {
	m, _ := liveModelWith(t, store.Node{NodeID: "1", Name: "ceo", Role: "lead",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"})
	v := m.View()
	for _, k := range []string{"h hire", "m msg", "x fire", "disband", "find"} {
		if !strings.Contains(v, k) {
			t.Fatalf("footer does not advertise %q", k)
		}
	}
}

// REGRESSION: pressing h on an EMPTY org must show the hire modal. The view
// short-circuited on a nil root and returned only the splash, so the modal
// opened invisibly and h looked dead. Every other interaction test seeded a
// node, so root was never nil and this went uncaught.
func TestHireModalVisibleOnEmptyOrg(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := NewLive(s, nil) // no nodes -> nil root -> empty state
	if m.root != nil {
		t.Fatal("precondition: expected empty org")
	}

	m = send(m, "h")
	if m.mode != modeHire {
		t.Fatalf("h did not open hire mode on an empty org (mode=%v)", m.mode)
	}
	m = send(m, "d", "e", "v")

	v := m.View()
	if !strings.Contains(v, "hire") || !strings.Contains(v, "dev") {
		t.Fatalf("hire modal not rendered over the empty state — h looks dead:\n%s", v)
	}
	if !strings.Contains(v, "esc cancel") {
		t.Fatalf("modal controls not shown on empty org:\n%s", v)
	}
}
