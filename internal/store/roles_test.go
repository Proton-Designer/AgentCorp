package store

import "testing"

func TestSeedDefaultRolesOnFreshStore(t *testing.T) {
	s := newTestStore(t)
	roles, err := s.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != len(DefaultRoles) {
		t.Fatalf("fresh store has %d roles, want %d defaults", len(roles), len(DefaultRoles))
	}
	if _, ok, _ := s.GetRole("researcher"); !ok {
		t.Fatal("default 'researcher' role missing")
	}
}

func TestUpsertRoleIsIdempotentEdit(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertRole(Role{Name: "custom", Glyph: "▲", Prompt: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRole(Role{Name: "custom", Glyph: "▲", Prompt: "v2"}); err != nil {
		t.Fatal(err)
	}
	r, ok, err := s.GetRole("custom")
	if err != nil || !ok {
		t.Fatalf("custom role missing: ok=%v err=%v", ok, err)
	}
	if r.Prompt != "v2" {
		t.Fatalf("upsert did not edit in place: prompt=%q", r.Prompt)
	}
	// Still exactly one 'custom' (defaults + 1).
	roles, _ := s.ListRoles()
	count := 0
	for _, r := range roles {
		if r.Name == "custom" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("custom role duplicated: %d rows", count)
	}
}

func TestSeedDoesNotClobberEdits(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertRole(Role{Name: "researcher", Glyph: "◆", Prompt: "my edited prompt"}); err != nil {
		t.Fatal(err)
	}
	// A second seed (as a re-open would do) must not overwrite the edit.
	if err := s.SeedDefaultRoles(); err != nil {
		t.Fatal(err)
	}
	r, _, _ := s.GetRole("researcher")
	if r.Prompt != "my edited prompt" {
		t.Fatalf("seed clobbered an operator edit: %q", r.Prompt)
	}
}

func TestGetRoleMissingIsNotError(t *testing.T) {
	s := newTestStore(t)
	if _, ok, err := s.GetRole("nope"); err != nil || ok {
		t.Fatalf("missing role: ok=%v err=%v, want false/nil", ok, err)
	}
}
