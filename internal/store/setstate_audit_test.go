package store

import (
	"strings"
	"testing"
)

// TestSetStateGuardHolds confirms the invariant SetState's own doc comment declares
// but nothing tested: it refuses 'alive' and 'dead', so no caller can bypass
// BindPeer's pending-only guard or resurrect a tombstoned node through the general
// setter. Surfaced by the epistemic auditor's boundary-coverage detector — a
// documented invariant with zero coverage, one edit away from silently breaking (the
// CI-canary shape). The guard holds today; this test makes a future break loud.
func TestSetStateGuardHolds(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("n1", "")); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// The two legal terminal transitions are accepted.
	for _, ok := range []string{"failed", "pending"} {
		if err := s.SetState("n1", ok); err != nil {
			t.Errorf("SetState(%q) should succeed, got %v", ok, err)
		}
	}

	// The guarded states and anything else are refused — this IS the invariant.
	// 'alive' and 'dead' each have exactly one guarded entry point (BindPeer /
	// Tombstone); the general setter must never be a back door to either.
	for _, bad := range []string{"alive", "dead", "zombie", ""} {
		err := s.SetState("n1", bad)
		if err == nil {
			t.Errorf("SetState(%q) must be refused (guarded transition / resurrection path), but it succeeded", bad)
			continue
		}
		if !strings.Contains(err.Error(), "refuses") {
			t.Errorf("SetState(%q) refused with an unexpected error: %v", bad, err)
		}
	}

	// A missing node still errors on an otherwise-legal state.
	if err := s.SetState("ghost", "failed"); err == nil {
		t.Error("SetState on a missing node should error")
	}
}
