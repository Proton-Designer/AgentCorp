package sync

import (
	"reflect"
	"testing"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/store"
)

func TestDecideTombstonesDeadNodes(t *testing.T) {
	recon := broker.Reconciliation{
		Dead: []store.Node{{NodeID: "n1"}, {NodeID: "n2"}},
	}
	got := Decide(PaneDiff{}, recon, nil)
	want := []string{"n1", "n2"}
	if !reflect.DeepEqual(got.Tombstone, want) {
		t.Fatalf("Tombstone = %v, want %v", got.Tombstone, want)
	}
}

func TestDecidePassesThroughPendingBinds(t *testing.T) {
	recon := broker.Reconciliation{
		PendingBind: []broker.Binding{{NodeID: "n1", PeerID: "p1"}},
	}
	got := Decide(PaneDiff{}, recon, nil)
	want := []broker.Binding{{NodeID: "n1", PeerID: "p1"}}
	if !reflect.DeepEqual(got.Bind, want) {
		t.Fatalf("Bind = %v, want %v", got.Bind, want)
	}
}

func TestDecideEmptyReconciliationIsQuiet(t *testing.T) {
	got := Decide(PaneDiff{}, broker.Reconciliation{}, nil)
	if len(got.Tombstone) != 0 || len(got.Bind) != 0 {
		t.Fatalf("expected no actions, got %+v", got)
	}
}

// The broker hasn't reaped this peer yet (its own sweep runs on its own
// cadence), but the tmux pane already vanished -- pane-diff is often FASTER
// than the broker signal, which is the whole reason it's a second signal
// and not a redundant one.
func TestDecideTombstonesViaPaneDeathSignalAloneWhenBrokerHasNotCaughtUp(t *testing.T) {
	diff := PaneDiff{Died: []string{"%3"}}
	nodes := []store.Node{{NodeID: "n1", SpawnRef: "%3", State: "alive"}}
	got := Decide(diff, broker.Reconciliation{}, nodes) // recon.Dead empty -- broker doesn't know yet
	if !reflect.DeepEqual(got.Tombstone, []string{"n1"}) {
		t.Fatalf("Tombstone = %v, want [n1] (pane-diff signal alone must be sufficient)", got.Tombstone)
	}
}

// The corollary: a node that never had a pane must never be tombstoned by
// the pane-diff signal, no matter what shows up in Died. This is the one
// most likely to be broken by a careless "spawn_ref == died pane" match if
// empty string is ever accidentally treated as a real identifier.
func TestDecideNeverTombstonesEmptySpawnRefViaPaneDiff(t *testing.T) {
	diff := PaneDiff{Died: []string{""}} // pathological, but must still be refused
	nodes := []store.Node{{NodeID: "adopted-node", SpawnRef: "", State: "alive"}}
	got := Decide(diff, broker.Reconciliation{}, nodes)
	if len(got.Tombstone) != 0 {
		t.Fatalf("Tombstone = %v, want none -- a spawn_ref-less node was never eligible for the pane signal", got.Tombstone)
	}
}

// Both signals firing for the same node must not produce a duplicate
// tombstone entry.
func TestDecideDoesNotDuplicateWhenBothSignalsFireForSameNode(t *testing.T) {
	diff := PaneDiff{Died: []string{"%3"}}
	recon := broker.Reconciliation{Dead: []store.Node{{NodeID: "n1"}}}
	nodes := []store.Node{{NodeID: "n1", SpawnRef: "%3", State: "alive"}}
	got := Decide(diff, recon, nodes)
	if !reflect.DeepEqual(got.Tombstone, []string{"n1"}) {
		t.Fatalf("Tombstone = %v, want exactly one [n1], not a duplicate", got.Tombstone)
	}
}

// A node already tombstoned (state=dead) whose old pane_id happens to
// reappear in a later Died list (unlikely, but tmux pane_ids aren't proven
// globally unique across the process lifetime) must not be re-added.
func TestDecideSkipsAlreadyDeadNodesForPaneSignal(t *testing.T) {
	diff := PaneDiff{Died: []string{"%3"}}
	nodes := []store.Node{{NodeID: "n1", SpawnRef: "%3", State: "dead"}}
	got := Decide(diff, broker.Reconciliation{}, nodes)
	if len(got.Tombstone) != 0 {
		t.Fatalf("Tombstone = %v, want none -- already dead", got.Tombstone)
	}
}
