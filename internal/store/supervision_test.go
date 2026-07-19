package store

import (
	"testing"
	"time"
)

func TestInsertAndListRestartEvents(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("a", "")); err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	if err := s.InsertRestartEvent("a", t1, "pane died"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRestartEvent("a", t2, "peer vanished"); err != nil {
		t.Fatal(err)
	}

	events, err := s.ListRestartEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	// Oldest first.
	if !events[0].At.Equal(t1) || !events[1].At.Equal(t2) {
		t.Fatalf("events not oldest-first: %+v", events)
	}
	if events[0].NodeID != "a" || events[1].NodeID != "a" {
		t.Fatalf("wrong node id: %+v", events)
	}
}

func TestRestartEventsSurviveNodeDeletion(t *testing.T) {
	// The whole point of no FK on restarts: a fired node's history must
	// survive its own row being hard-deleted (fire/disband).
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("a", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRestartEvent("a", time.Now(), "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNode("a"); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListRestartEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("restart history did not survive node deletion: got %d events, want 1", len(events))
	}
}

func TestUpsertAndGetSupervisionPolicy(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("boss", "")); err != nil {
		t.Fatal(err)
	}

	p := Policy{NodeID: "boss", Strategy: OneForAll, MaxRestarts: 5, WindowSeconds: 600}
	if err := s.UpsertSupervisionPolicy(p); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.GetSupervisionPolicy("boss")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("want ok=true, policy was inserted")
	}
	if got != p {
		t.Fatalf("got %+v, want %+v", got, p)
	}
}

func TestGetSupervisionPolicyMissingIsNotAnError(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("boss", "")); err != nil {
		t.Fatal(err)
	}
	_, ok, err := s.GetSupervisionPolicy("boss")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("want ok=false, no policy was ever set — this is a normal outcome, not an error")
	}
}

func TestUpsertSupervisionPolicyIsIdempotentEdit(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("boss", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSupervisionPolicy(Policy{NodeID: "boss", Strategy: OneForOne, MaxRestarts: 3, WindowSeconds: 300}); err != nil {
		t.Fatal(err)
	}
	// Re-run with the same node_id, different values -- must edit, not duplicate.
	if err := s.UpsertSupervisionPolicy(Policy{NodeID: "boss", Strategy: RestForOne, MaxRestarts: 10, WindowSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetSupervisionPolicy("boss")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Strategy != RestForOne || got.MaxRestarts != 10 {
		t.Fatalf("got %+v, want the edited values (rest-for-one, 10)", got)
	}
	all, err := s.ListSupervisionPolicies()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("upsert duplicated the row: got %d policies, want 1", len(all))
	}
}

func TestSupervisionPolicyFKRejectsUnknownNode(t *testing.T) {
	s := newTestStore(t)
	// supervision_policy DOES FK to nodes (unlike restarts) -- it's live
	// config, meaningless once the node doesn't exist.
	err := s.UpsertSupervisionPolicy(Policy{NodeID: "does-not-exist", Strategy: OneForOne, MaxRestarts: 3, WindowSeconds: 300})
	if err == nil {
		t.Fatal("want an FK violation for a policy on a nonexistent node, got nil")
	}
}

func TestDeleteSupervisionPolicyRevertsToDefault(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertNode(mkNode("boss", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSupervisionPolicy(Policy{NodeID: "boss", Strategy: OneForAll, MaxRestarts: 5, WindowSeconds: 600}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSupervisionPolicy("boss"); err != nil {
		t.Fatal(err)
	}
	_, ok, err := s.GetSupervisionPolicy("boss")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("policy should be gone after delete")
	}
}

func TestListSupervisionPoliciesEmptyIsNilNotError(t *testing.T) {
	s := newTestStore(t)
	policies, err := s.ListSupervisionPolicies()
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 0 {
		t.Fatalf("want no policies on a fresh store, got %+v", policies)
	}
}
