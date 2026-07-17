package sync

import (
	"reflect"
	"sort"
	"testing"
)

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestDiffPanesDetectsDeath(t *testing.T) {
	prev := map[string]Pane{"%1": {ID: "%1", PID: "100"}, "%2": {ID: "%2", PID: "200"}}
	cur := map[string]Pane{"%1": {ID: "%1", PID: "100"}} // %2 vanished -- the default-tmux death signal
	d := DiffPanes(prev, cur)
	if !reflect.DeepEqual(sorted(d.Died), []string{"%2"}) {
		t.Fatalf("Died = %v, want [%%2]", d.Died)
	}
	if len(d.New) != 0 {
		t.Fatalf("New = %v, want none", d.New)
	}
}

func TestDiffPanesDetectsNewPane(t *testing.T) {
	prev := map[string]Pane{"%1": {ID: "%1", PID: "100"}}
	cur := map[string]Pane{"%1": {ID: "%1", PID: "100"}, "%2": {ID: "%2", PID: "200"}}
	d := DiffPanes(prev, cur)
	if len(d.Died) != 0 {
		t.Fatalf("Died = %v, want none", d.Died)
	}
	if !reflect.DeepEqual(sorted(d.New), []string{"%2"}) {
		t.Fatalf("New = %v, want [%%2]", d.New)
	}
}

func TestDiffPanesNoChangeIsQuiet(t *testing.T) {
	prev := map[string]Pane{"%1": {ID: "%1", PID: "100"}}
	cur := map[string]Pane{"%1": {ID: "%1", PID: "100"}}
	d := DiffPanes(prev, cur)
	if len(d.Died) != 0 || len(d.New) != 0 {
		t.Fatalf("expected no changes, got %+v", d)
	}
}

// PID reuse: same pane_id, different pane_pid. This is a real tmux edge case
// (unlikely but not impossible if a pane_id is somehow recycled) and the diff
// must not treat it as "unchanged" just because the map key matches.
func TestDiffPanesDetectsPIDChangeUnderSamePaneID(t *testing.T) {
	prev := map[string]Pane{"%1": {ID: "%1", PID: "100"}}
	cur := map[string]Pane{"%1": {ID: "%1", PID: "999"}}
	d := DiffPanes(prev, cur)
	if len(d.Died) != 1 || d.Died[0] != "%1" {
		t.Fatalf("Died = %v, want [%%1] (pid changed under same pane_id)", d.Died)
	}
	if len(d.New) != 1 || d.New[0] != "%1" {
		t.Fatalf("New = %v, want [%%1] (treated as a fresh pane)", d.New)
	}
}

func TestDiffPanesMultipleSimultaneousDeaths(t *testing.T) {
	prev := map[string]Pane{
		"%1": {ID: "%1", PID: "1"}, "%2": {ID: "%2", PID: "2"}, "%3": {ID: "%3", PID: "3"},
	}
	cur := map[string]Pane{"%2": {ID: "%2", PID: "2"}}
	d := DiffPanes(prev, cur)
	if !reflect.DeepEqual(sorted(d.Died), []string{"%1", "%3"}) {
		t.Fatalf("Died = %v, want [%%1 %%3]", d.Died)
	}
}

func TestDiffPanesEmptyToEmpty(t *testing.T) {
	d := DiffPanes(map[string]Pane{}, map[string]Pane{})
	if len(d.Died) != 0 || len(d.New) != 0 {
		t.Fatalf("expected no changes on empty->empty, got %+v", d)
	}
}

func TestDiffPanesNilMapsAreSafe(t *testing.T) {
	// A nil map (e.g. from a zero-value Pane snapshot before the first tick)
	// must behave like an empty one, not panic.
	d := DiffPanes(nil, map[string]Pane{"%1": {ID: "%1", PID: "1"}})
	if !reflect.DeepEqual(sorted(d.New), []string{"%1"}) {
		t.Fatalf("New = %v, want [%%1]", d.New)
	}
	d2 := DiffPanes(map[string]Pane{"%1": {ID: "%1", PID: "1"}}, nil)
	if !reflect.DeepEqual(sorted(d2.Died), []string{"%1"}) {
		t.Fatalf("Died = %v, want [%%1]", d2.Died)
	}
}
