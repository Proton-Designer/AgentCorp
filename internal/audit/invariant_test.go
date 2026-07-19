package audit

import "testing"

func TestScanInvariantsFlagsUnattested(t *testing.T) {
	src := map[string]string{
		"nodes.go": `package store

// SetState refuses 'alive' and 'dead' — those transitions are guarded elsewhere.
func SetState(id, s string) error { return nil }

// helper does a small thing.
func helper() {}

// BindPeer must reject a double-bind.
func BindPeer(id string) error { return nil }
`,
	}
	// Only BindPeer is referenced by a test; SetState is not.
	tests := map[string]string{
		"nodes_test.go": `package store
func TestBind(t *testing.T) { BindPeer("x") }`,
	}
	got := ScanInvariants(LangGo, src, tests)
	if len(got) != 1 {
		t.Fatalf("expected exactly SetState flagged, got %d: %+v", len(got), got)
	}
	if got[0].Symbol != "SetState" {
		t.Errorf("wrong symbol flagged: %q", got[0].Symbol)
	}
	if got[0].Keyword != "refuses" {
		t.Errorf("wrong keyword: %q", got[0].Keyword)
	}
}

func TestScanInvariantsIgnoresAttestedAndPlain(t *testing.T) {
	src := map[string]string{
		"x.go": `package x

// Always positive is guaranteed here.
func Positive() int { return 1 }
`,
	}
	// Positive IS referenced by a test → attested (necessary, not sufficient — but
	// the detector's job is only to find the UN-referenced ones).
	tests := map[string]string{"x_test.go": `package x
func TestPos(t *testing.T){ Positive() }`}
	if got := ScanInvariants(LangGo, src, tests); len(got) != 0 {
		t.Errorf("an invariant whose symbol a test names must not be flagged, got %+v", got)
	}
}

func TestScanInvariantsTypeScript(t *testing.T) {
	src := map[string]string{
		"render.ts": `
// A cell mode never promotes from INDETERMINATE to ALIGNED without calibration.
export function classifyCell(x: number): string { return "INDETERMINATE"; }
`,
	}
	got := ScanInvariants(LangTS, src, map[string]string{})
	if len(got) != 1 || got[0].Symbol != "classifyCell" {
		t.Fatalf("TS invariant comment should flag classifyCell, got %+v", got)
	}
}
