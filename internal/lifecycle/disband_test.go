package lifecycle

import (
	"testing"
	"time"

	"github.com/aymanmohammed/crew/internal/store"
)

// indexOf returns the position of id in the ordered result, or -1.
func indexOf(nodes []string, id string) int {
	for i, n := range nodes {
		if n == id {
			return i
		}
	}
	return -1
}

func idsOf(nodes []store.Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.NodeID)
	}
	return out
}

func TestDisbandKillsChildrenBeforeParent(t *testing.T) {
	nodes := []store.Node{
		node("root", "", "alive"),
		node("a", "root", "alive"),
		node("b", "root", "alive"),
		node("a1", "a", "alive"),
		node("a2", "a", "alive"),
	}
	got, err := Disband(nodes, "root")
	if err != nil {
		t.Fatalf("Disband: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d nodes, want 5 (whole subtree, root included): %+v", len(got), got)
	}

	ids := idsOf(got)

	// The invariant that actually matters: every node appears AFTER all of
	// its own descendants. Cross-branch ordering (e.g. b vs a1) is not
	// constrained -- b and a1 are unrelated, so killing either first is fine.
	if indexOf(ids, "a1") > indexOf(ids, "a") || indexOf(ids, "a2") > indexOf(ids, "a") {
		t.Fatalf("a's children must be killed before a itself, got order %v", ids)
	}
	if indexOf(ids, "a") > indexOf(ids, "root") || indexOf(ids, "b") > indexOf(ids, "root") {
		t.Fatalf("root's children must be killed before root itself, got order %v", ids)
	}
	if ids[len(ids)-1] != "root" {
		t.Fatalf("root (the shallowest node in its own subtree) must be killed last, got order %v", ids)
	}
}

func TestDisbandSingleNodeNoChildren(t *testing.T) {
	got, err := Disband([]store.Node{node("solo", "", "alive")}, "solo")
	if err != nil {
		t.Fatalf("Disband: %v", err)
	}
	if len(got) != 1 || got[0].NodeID != "solo" {
		t.Fatalf("Disband = %+v, want [solo]", got)
	}
}

// A pending node (never bound to a real peer) must still be included in the
// disband decision -- it's part of the subtree structurally, even though
// there's no live process to SIGTERM for it. What to actually do with a
// pending node operationally is the caller's job; Disband's only obligation
// is to not skip or error on it.
func TestDisbandIncludesPendingNode(t *testing.T) {
	nodes := []store.Node{
		node("root", "", "alive"),
		node("pending-child", "root", "pending"),
	}
	got, err := Disband(nodes, "root")
	if err != nil {
		t.Fatalf("Disband: %v", err)
	}
	if indexOf(idsOf(got), "pending-child") == -1 {
		t.Fatalf("Disband omitted a pending node: %+v", got)
	}
}

func TestDisbandUnknownRootErrors(t *testing.T) {
	if _, err := Disband([]store.Node{node("a", "", "alive")}, "nope"); err == nil {
		t.Fatal("expected an error for an unknown root")
	}
}

// A cycle in parent_id in the DOWNWARD direction (a node that is, through
// its children pointers, transitively its own descendant) must not hang a
// naive recursive walk. Proven with a hard deadline, same as the Reparent
// cycle test.
func TestDisbandCycleTerminates(t *testing.T) {
	nodes := []store.Node{
		node("a", "b", "alive"), // a's parent is b
		node("b", "a", "alive"), // b's parent is a -- so walking "children of a" finds b, "children of b" finds a again
	}

	done := make(chan struct{})
	var err error
	go func() {
		_, err = Disband(nodes, "a")
		close(done)
	}()

	select {
	case <-done:
		if err == nil {
			t.Fatal("expected a cycle-detected error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Disband did not terminate within 2s -- likely infinite recursion on the cyclic subtree")
	}
}
