package audit

import (
	"testing"
	"time"
)

func TestBoundaryEqual(t *testing.T) {
	// A measurement equal to its cap fires; a measurement below it does not.
	if f := BoundaryEqual(Check{ID: "m", Result: F(20.5), Cap: F(20.5), Unit: "s"}); f == nil {
		t.Errorf("result==cap should fire boundary-equal")
	} else if f.Signal != SignalBoundaryEqual {
		t.Errorf("wrong signal %q", f.Signal)
	}
	if f := BoundaryEqual(Check{ID: "m", Result: F(12.0), Cap: F(20.5)}); f != nil {
		t.Errorf("result below cap must not fire, got %v", f)
	}
	// Missing either field: silent (nothing to compare — not a pass, just no signal).
	if f := BoundaryEqual(Check{ID: "m", Result: F(20.5)}); f != nil {
		t.Errorf("no cap → no comparison → no finding")
	}
}

func TestNonDiscriminating(t *testing.T) {
	// A check that passes on a seeded break is a fig leaf.
	if f := NonDiscriminating(Check{ID: "t", FailsOnBreak: B(false)}); f == nil {
		t.Errorf("a check that passes on a break must be flagged")
	}
	// A check that fails on a break discriminates — clean.
	if f := NonDiscriminating(Check{ID: "t", FailsOnBreak: B(true)}); f != nil {
		t.Errorf("a discriminating check must not be flagged, got %v", f)
	}
	// Never probed: no claim either way.
	if f := NonDiscriminating(Check{ID: "t"}); f != nil {
		t.Errorf("an unprobed check must not be flagged (absence ≠ fig leaf)")
	}
}

func TestCostSurprise(t *testing.T) {
	// Ran in 5ms against a predicted 1s — implausibly cheap.
	if f := CostSurprise(Check{ID: "t", Duration: 5 * time.Millisecond, Predicted: time.Second}); f == nil {
		t.Errorf("a check far faster than predicted should fire")
	}
	// Ran about as long as predicted — clean.
	if f := CostSurprise(Check{ID: "t", Duration: 900 * time.Millisecond, Predicted: time.Second}); f != nil {
		t.Errorf("a check near its prediction must not fire, got %v", f)
	}
	// No prediction: the detector has no baseline and stays silent.
	if f := CostSurprise(Check{ID: "t", Duration: time.Millisecond}); f != nil {
		t.Errorf("no prediction → no baseline → no finding")
	}
}

func TestCorrelatedProvenance(t *testing.T) {
	a := Check{ID: "a", Agrees: "b", Deps: []string{"go-runewidth", "unicode"}}
	b := Check{ID: "b", Deps: []string{"go-runewidth"}}
	if f := CorrelatedProvenance(a, b); f == nil {
		t.Errorf("agreeing checks sharing a dependency must be flagged")
	}
	// Disjoint dependencies: agreement is real evidence, not flagged.
	c := Check{ID: "c", Agrees: "d", Deps: []string{"x"}}
	d := Check{ID: "d", Deps: []string{"y"}}
	if f := CorrelatedProvenance(c, d); f != nil {
		t.Errorf("disjoint-derivation agreement is genuine evidence, got %v", f)
	}
	// Sharing a dep but NOT claiming to corroborate: not flagged (no agreement asserted).
	e := Check{ID: "e", Deps: []string{"z"}}
	g := Check{ID: "g", Deps: []string{"z"}}
	if f := CorrelatedProvenance(e, g); f != nil {
		t.Errorf("shared dep without an agreement claim is not a correlated corroboration")
	}
}

func TestMissingBoundaryCoverage(t *testing.T) {
	caps := []Capability{
		{Name: "hire", ReachedByRealTest: true},
		{Name: "changedSince", ReachedByRealTest: false},
	}
	fs := MissingBoundaryCoverage(caps)
	if len(fs) != 1 || fs[0].CheckID != "changedSince" {
		t.Fatalf("only the unreached capability should be flagged, got %v", fs)
	}
	if fs[0].Signal != SignalMissingBoundary {
		t.Errorf("wrong signal %q", fs[0].Signal)
	}
}

func TestRunDeterministic(t *testing.T) {
	checks := []Check{
		{ID: "1", Result: F(20.5), Cap: F(20.5)},
		{ID: "2", FailsOnBreak: B(false)},
	}
	a := Run(checks)
	b := Run(checks)
	if len(a) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("Run must be deterministic; differed at %d", i)
		}
	}
}
