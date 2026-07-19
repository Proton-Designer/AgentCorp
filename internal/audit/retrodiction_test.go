package audit

import (
	"sort"
	"strings"
	"testing"
)

// This is the auditor's own falsifier (docs/design/proof-carrying-claims.md §5).
// It encodes real, ground-truth honest-error failures and measures which detectors
// would have fired — mechanically, by running the actual detector code over fields
// set to what was true at the time, NOT by hand-labelling the outcome. Some cases
// fire nothing; those are recorded, not hidden. Pre-registered prediction (written
// before this ran): ~9–10 of 14 caught, with the byte-vs-rune miscount, the
// wrong-path check, and the bare-assertion phantoms predicted as MISSES.
//
// Confound notes, stated plainly:
//   - Encoding from a known answer is itself hindsight-biased (an encoder can pick
//     fields that make a detector fire). Mitigation: fields are set to what actually
//     happened; the detector FUNCTIONS decide, not the labeller. It remains weaker
//     than a prospective run (see prober_test.go / the live audit command).
//   - "agentcorp-lived" cases were experienced first-hand in this project and are
//     encodable from artifacts. "mcp-narrative" cases are second-hand from a peer's
//     report and are weaker evidence; both are marked.

type failureCase struct {
	id        string
	desc      string
	source    string // "agentcorp-lived" | "mcp-narrative"
	predicted Signal // pre-registered guess; "" means predicted MISS
	// Exactly one encoding is populated:
	check *Check
	pair  *[2]Check
	cap   *Capability
}

func retrodictionSet() []failureCase {
	return []failureCase{
		// --- AgentCorp, lived first-hand ---
		{
			id: "width-phantom", source: "agentcorp-lived", predicted: "",
			desc:  "claimed a width bug that was inferred from a code comment, never observed",
			check: &Check{ID: "width-phantom", Claim: "the chart has a width bug"}, // no verification existed at all
		},
		{
			id: "stale-binary", source: "agentcorp-lived", predicted: "",
			desc:  "claimed the built binary was current; it predated the fixes. Cheap falsifier (mtime bin>src) existed but was never run",
			check: &Check{ID: "stale-binary", Claim: "the binary is current"}, // the falsifier existed but was not invoked — no check to inspect
		},
		{
			id: "byte-vs-rune", source: "agentcorp-lived", predicted: "",
			desc:  "measured chart centering in BYTES (wc -c); box-drawing is 3 bytes/cell, so correct centering read as asymmetric",
			check: &Check{ID: "byte-vs-rune", Claim: "the chart is off-center", Result: F(56), Cap: F(46), Unit: "bytes"}, // wrong-unit measurement; no cap involved, 56≠46 so boundary won't fire
		},
		{
			id: "colorblind-md5", source: "agentcorp-lived", predicted: SignalNonDiscriminating,
			desc: "concluded 'animation not rendering' from md5 of a colour-stripped capture (capture-pane without -e); the check is blind to colour-only changes",
			// The md5 stayed identical even though the animation WAS present → does not
			// fail when the thing it checks for is present.
			check: &Check{ID: "colorblind-md5", Claim: "the tree animation is not rendering", FailsOnBreak: B(false)},
		},

		// --- terminal-MCP team, second-hand narrative ---
		{
			id: "loop-exhaustion-20.5s", source: "mcp-narrative", predicted: SignalBoundaryEqual,
			desc:  "a 20.5s 'measurement' that equalled the polling loop's own exhaustion budget",
			check: &Check{ID: "loop-exhaustion-20.5s", Claim: "operation took 20.5s", Result: F(20.5), Cap: F(20.5), Unit: "s"},
		},
		{
			id: "canary-gate", source: "mcp-narrative", predicted: SignalNonDiscriminating,
			desc:  "a CI gate 'protecting' a canary that had never been observed failing — could not detect the canary being deleted",
			check: &Check{ID: "canary-gate", Claim: "CI protects the canary", FailsOnBreak: B(false)},
		},
		{
			id: "async-race-test", source: "mcp-narrative", predicted: SignalNonDiscriminating,
			desc:  "a regression test that passed with the regression reintroduced — it raced an async effect and checked before the effect occurred",
			check: &Check{ID: "async-race-test", Claim: "the regression is fixed", FailsOnBreak: B(false)},
		},
		{
			id: "eaw-width-corroboration", source: "mcp-narrative", predicted: SignalCorrelatedProvenance,
			desc: "two renderers 'agreeing' a glyph is one cell wide — because neither implements ambiguous-width",
			pair: &[2]Check{
				{ID: "tmux-width", Claim: "glyph is width 1", Agrees: "xterm-width", Deps: []string{"no-eaw-implementation"}},
				{ID: "xterm-width", Claim: "glyph is width 1", Deps: []string{"no-eaw-implementation"}},
			},
		},
		{
			id: "changedSince-unreachable", source: "mcp-narrative", predicted: SignalMissingBoundary,
			desc: "a feature implemented, unit-tested in-process, and unreachable over the wire — no test crossed the transport boundary",
			cap:  &Capability{Name: "changedSince", ReachedByRealTest: false},
		},
		{
			id: "overflow-wrong-half", source: "mcp-narrative", predicted: SignalNonDiscriminating,
			desc:  "an overflow detector that scanned the wrong half of the buffer, so it never once returned true",
			check: &Check{ID: "overflow-wrong-half", Claim: "the renderer never clips", FailsOnBreak: B(false)},
		},
		{
			id: "wrong-path-checkignore", source: "mcp-narrative", predicted: "",
			desc:  "a git check-ignore run against the wrong resolved path — green, and meaningless (wrong TARGET, not a broken check)",
			check: &Check{ID: "wrong-path-checkignore", Claim: "the path is ignored"}, // nothing a detector inspects
		},
		{
			id: "changedSince-judgment", source: "mcp-narrative", predicted: SignalMissingBoundary,
			desc: "the judgment 'this closes the gap', whose cheap shadow (is it reachable?) was a boundary-crossing test that did not exist",
			cap:  &Capability{Name: "changedSince-closes-gap", ReachedByRealTest: false},
		},
		{
			id: "self-contradictory-timestamp", source: "mcp-narrative", predicted: "",
			desc:  "a build timestamp asserted that contradicted itself — internal inconsistency, which no detector targets",
			check: &Check{ID: "self-contradictory-timestamp", Claim: "the binary was built at 13:54"}, // nothing to inspect
		},
		{
			id: "proxy2-false-corroboration", source: "agentcorp-lived", predicted: SignalCorrelatedProvenance,
			desc: "mid-design, treated a recollection and a web search as two independent sources — both derive from the same published literature",
			pair: &[2]Check{
				{ID: "recollection", Claim: "the citation numbers are X", Agrees: "search", Deps: []string{"published-literature"}},
				{ID: "search", Claim: "the citation numbers are X", Deps: []string{"published-literature"}},
			},
		},
	}
}

// firedSignals runs the real detectors over a case's encoding and returns the
// signals that fired.
func (fc failureCase) firedSignals() []Signal {
	var out []Signal
	switch {
	case fc.check != nil:
		for _, f := range Run([]Check{*fc.check}) {
			out = append(out, f.Signal)
		}
	case fc.pair != nil:
		if f := CorrelatedProvenance(fc.pair[0], fc.pair[1]); f != nil {
			out = append(out, f.Signal)
		}
		// per-check detectors too, in case a pair member also trips one
		for _, f := range Run(fc.pair[:]) {
			if f.Signal != SignalCorrelatedProvenance {
				out = append(out, f.Signal)
			}
		}
	case fc.cap != nil:
		for _, f := range MissingBoundaryCoverage([]Capability{*fc.cap}) {
			out = append(out, f.Signal)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestRetrodiction(t *testing.T) {
	cases := retrodictionSet()
	caught, predictedMatch := 0, 0
	var lines []string
	for _, fc := range cases {
		fired := fc.firedSignals()
		hit := len(fired) > 0
		if hit {
			caught++
		}
		// Did the outcome match the pre-registered prediction?
		match := (fc.predicted == "" && !hit) || (fc.predicted != "" && containsSig(fired, fc.predicted))
		if match {
			predictedMatch++
		}
		firedStr := "—"
		if hit {
			ss := make([]string, len(fired))
			for i, s := range fired {
				ss[i] = string(s)
			}
			firedStr = strings.Join(ss, ",")
		}
		pred := string(fc.predicted)
		if pred == "" {
			pred = "MISS"
		}
		lines = append(lines, "  "+padr(fc.id, 30)+" "+padr(fc.source, 16)+" fired="+padr(firedStr, 24)+" predicted="+padr(pred, 24)+boolMark(match))
	}

	t.Logf("\nRETRODICTION — %d/%d caught by ≥1 detector; %d/%d matched pre-registration\n%s",
		caught, len(cases), predictedMatch, len(cases), strings.Join(lines, "\n"))

	// Weak sanity floor: the cases the design is CONFIDENT about must fire, so a
	// detector regression breaks this test. Deliberately not asserting the exact
	// count — that would be gaming the pre-registered prediction.
	mustFire := map[string]Signal{
		"loop-exhaustion-20.5s":    SignalBoundaryEqual,
		"canary-gate":              SignalNonDiscriminating,
		"eaw-width-corroboration":  SignalCorrelatedProvenance,
		"changedSince-unreachable": SignalMissingBoundary,
	}
	byID := map[string]failureCase{}
	for _, fc := range cases {
		byID[fc.id] = fc
	}
	for id, sig := range mustFire {
		if !containsSig(byID[id].firedSignals(), sig) {
			t.Errorf("regression: %q must fire %q but did not", id, sig)
		}
	}
}

func containsSig(ss []Signal, want Signal) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func padr(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func boolMark(b bool) string {
	if b {
		return " ✓"
	}
	return " ✗"
}
