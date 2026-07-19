package audit

import "testing"

func TestFilterUnreached(t *testing.T) {
	findings := []InvariantFinding{
		{Symbol: "SetState"},     // 0% → kept (unreached, high confidence)
		{Symbol: "encodeX10"},    // reached through a public entry point → dropped
		{Symbol: "NeverCovered"}, // absent from profile → kept
	}
	cov := map[string]float64{
		"SetState":  0.0,
		"encodeX10": 87.5,
		"unrelated": 100.0,
	}
	kept, dropped := FilterUnreached(findings, cov)
	if len(kept) != 2 {
		t.Fatalf("expected 2 kept (SetState, NeverCovered), got %d: %+v", len(kept), kept)
	}
	if len(dropped) != 1 || dropped[0].Symbol != "encodeX10" {
		t.Fatalf("only the reached symbol should be dropped, got %+v", dropped)
	}
}
