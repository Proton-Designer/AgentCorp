package lifecycle

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/aymanmohammed/crew/internal/store"
)

func node(id, parent, state string) store.Node {
	return store.Node{NodeID: id, ParentID: parent, Name: id, Role: "dev",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: state, CreatedAt: "t"}
}

func sortedMoves(m []Move) []Move {
	out := append([]Move(nil), m...)
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

func TestReparentMovesChildrenToGrandparent(t *testing.T) {
	nodes := []store.Node{
		node("g", "", "alive"),
		node("victim", "g", "alive"),
		node("c1", "victim", "alive"),
		node("c2", "victim", "alive"),
	}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	want := []Move{
		{NodeID: "c1", OldParentID: "victim", NewParentID: "g"},
		{NodeID: "c2", OldParentID: "victim", NewParentID: "g"},
	}
	if !reflect.DeepEqual(sortedMoves(got), sortedMoves(want)) {
		t.Fatalf("Reparent = %+v, want %+v", got, want)
	}
}

func TestReparentVictimIsRootChildrenBecomeRoots(t *testing.T) {
	nodes := []store.Node{
		node("victim", "", "alive"),
		node("c1", "victim", "alive"),
	}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	want := []Move{{NodeID: "c1", OldParentID: "victim", NewParentID: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reparent = %+v, want %+v", got, want)
	}
}

// The case the brief specifically calls out: reparenting a node whose own
// parent is ALSO being fired (already tombstoned in the given snapshot, per
// the tombstone-not-prune policy the row is retained). Children must walk
// past the dead ancestor to the nearest LIVE one, not be reattached under a
// dead node that can't actually manage them.
func TestReparentSkipsDeadAncestorsToNearestLiveOne(t *testing.T) {
	nodes := []store.Node{
		node("g", "", "alive"),          // live grandparent
		node("deadParent", "g", "dead"), // already fired, tombstoned, row retained
		node("victim", "deadParent", "alive"),
		node("c1", "victim", "alive"),
	}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	want := []Move{{NodeID: "c1", OldParentID: "victim", NewParentID: "g"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reparent = %+v, want %+v (should skip dead ancestor and land on live grandparent)", got, want)
	}
}

// Multiple dead ancestors in a row must all be skipped.
func TestReparentSkipsMultipleDeadAncestors(t *testing.T) {
	nodes := []store.Node{
		node("g", "", "alive"),
		node("dead1", "g", "dead"),
		node("dead2", "dead1", "dead"),
		node("victim", "dead2", "alive"),
		node("c1", "victim", "alive"),
	}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	want := []Move{{NodeID: "c1", OldParentID: "victim", NewParentID: "g"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reparent = %+v, want %+v", got, want)
	}
}

// If EVERY ancestor up to the top is dead, children become roots -- there's
// no live node left to attach them to.
func TestReparentAllAncestorsDeadFallsBackToRoot(t *testing.T) {
	nodes := []store.Node{
		node("dead1", "", "dead"),
		node("victim", "dead1", "alive"),
		node("c1", "victim", "alive"),
	}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	want := []Move{{NodeID: "c1", OldParentID: "victim", NewParentID: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reparent = %+v, want %+v", got, want)
	}
}

func TestReparentNoChildrenIsQuiet(t *testing.T) {
	nodes := []store.Node{node("victim", "", "alive")}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Reparent = %+v, want no moves", got)
	}
}

func TestReparentUnknownVictimErrors(t *testing.T) {
	nodes := []store.Node{node("a", "", "alive")}
	if _, err := Reparent(nodes, "nope"); err == nil {
		t.Fatal("expected an error for an unknown victim")
	}
}

// A parent_id chain that runs off the edge of the given node set (shouldn't
// happen under FK enforcement in the real DB, but this is a pure function
// operating on whatever slice it's handed) must be treated as reaching a
// root, not crash or loop.
func TestReparentDanglingAncestorFallsBackToRoot(t *testing.T) {
	nodes := []store.Node{
		node("victim", "ghost-parent-not-in-slice", "alive"),
		node("c1", "victim", "alive"),
	}
	got, err := Reparent(nodes, "victim")
	if err != nil {
		t.Fatalf("Reparent: %v", err)
	}
	want := []Move{{NodeID: "c1", OldParentID: "victim", NewParentID: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reparent = %+v, want %+v", got, want)
	}
}

// A cycle in parent_id must produce an error, not a hang. Proven, not
// assumed: the actual call runs in a goroutine with a hard deadline, so a
// regression that reintroduces unbounded recursion fails this test by timing
// out rather than by silently passing on faith in the implementation.
func TestReparentCycleInAncestorChainTerminates(t *testing.T) {
	nodes := []store.Node{
		node("a", "b", "dead"), // a's parent is b
		node("b", "a", "dead"), // b's parent is a -- cycle
		node("victim", "a", "alive"),
		node("c1", "victim", "alive"),
	}

	done := make(chan struct{})
	var err error
	go func() {
		_, err = Reparent(nodes, "victim")
		close(done)
	}()

	select {
	case <-done:
		if err == nil {
			t.Fatal("expected a cycle-detected error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Reparent did not terminate within 2s -- likely an infinite loop on the cyclic parent chain")
	}
}
