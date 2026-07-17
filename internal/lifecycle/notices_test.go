package lifecycle

import (
	"strings"
	"testing"
)

func TestReparentNoticesOnePerMove(t *testing.T) {
	moves := []Move{
		{NodeID: "c1", OldParentID: "victim", NewParentID: "g"},
		{NodeID: "c2", OldParentID: "victim", NewParentID: "g"},
	}
	got := ReparentNotices(moves)
	if len(got) != 2 {
		t.Fatalf("got %d notices, want 2", len(got))
	}
	for i, m := range moves {
		if got[i].To != m.NodeID {
			t.Fatalf("notice %d.To = %q, want %q", i, got[i].To, m.NodeID)
		}
		if !strings.Contains(got[i].Message, m.NewParentID) {
			t.Fatalf("notice %d message %q doesn't mention new parent %q", i, got[i].Message, m.NewParentID)
		}
	}
}

// A move to the root (NewParentID == "") needs different wording -- naively
// formatting "you now report to %s" with an empty NewParentID would produce
// "you now report to " with nothing after it, a broken sentence.
func TestReparentNoticesRootCaseHasDistinctWording(t *testing.T) {
	got := ReparentNotices([]Move{{NodeID: "c1", OldParentID: "victim", NewParentID: ""}})
	if len(got) != 1 {
		t.Fatalf("got %d notices, want 1", len(got))
	}
	if strings.HasSuffix(strings.TrimSpace(got[0].Message), "report to") {
		t.Fatalf("root-case message reads like a broken sentence with nothing after 'report to': %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "root") {
		t.Fatalf("root-case message should clearly say the node is now a root: %q", got[0].Message)
	}
}

func TestReparentNoticesEmptyMovesIsQuiet(t *testing.T) {
	if got := ReparentNotices(nil); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}
