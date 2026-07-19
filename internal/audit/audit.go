// Package audit is the epistemic auditor: a small set of domain-general detectors
// that flag verification which could not have discriminated — a check that passes
// regardless of correctness, a measurement equal to its own cap, agreement between
// sources that share a blind spot, a capability no test reaches through the real
// path. See docs/design/proof-carrying-claims.md for the full design and the
// refutation that shaped it.
//
// The auditor answers "could this CHECK have caught anything," never the
// unanswerable "is this CLAIM true." Its detectors are NECESSARY, not sufficient:
// a check that survives all of them is not proven trustworthy — it has merely not
// been caught. There is deliberately no terminal "verified" state, and the output
// is FINDINGS to investigate, never a score to optimize (a scored detector is a
// gamed detector).
package audit

import (
	"fmt"
	"math"
	"time"
)

// Signal names a specific way a check could fail to discriminate. Recording which
// signal fired is an invariant: a bare "flagged" is itself unfalsifiable one level up.
type Signal string

const (
	// SignalBoundaryEqual: a measurement exactly equal to a known timeout/cap/budget/
	// bound — a clamped value posing as data (the strongest, hardest-to-fake detector).
	SignalBoundaryEqual Signal = "boundary-equal"
	// SignalNonDiscriminating: a check that provably does NOT fail when the thing it
	// verifies is deliberately broken — a fig leaf that passes regardless of correctness.
	SignalNonDiscriminating Signal = "non-discriminating"
	// SignalCostSurprise: a check that finished far cheaper/faster than its own logged
	// prediction (e.g. a test that returned before the async effect it checks could
	// occur). The weakest detector — needs a prediction, and variance mimics a shortcut.
	SignalCostSurprise Signal = "cost-surprise"
	// SignalCorrelatedProvenance: agreement between two checks whose derivations share a
	// dependency — agreement is evidence only if the paths are disjoint.
	SignalCorrelatedProvenance Signal = "correlated-provenance"
	// SignalMissingBoundary: a capability with no test reaching it through the real
	// transport — absence of a check, invisible to every other detector.
	SignalMissingBoundary Signal = "missing-boundary-coverage"
)

// Check is one verification the mesh produced, in a form the detectors can inspect.
// Every field is optional; a detector whose inputs are absent simply does not fire.
// Absence of a field is never itself a pass — a check with nothing to inspect is a
// check the auditor could not audit, which the caller surfaces rather than greenlights.
type Check struct {
	ID    string
	Claim string // what the check purports to verify (for the report, not the logic)

	// Boundary-equal inputs.
	Result *float64 // the numeric value the check produced, if any
	Cap    *float64 // a timeout/cap/budget/bound the instrument could have clamped to
	Unit   string   // for the report, e.g. "s", "iters"

	// Non-discriminating input: did the check FAIL when its target was deliberately
	// broken? nil = never probed; false = fig leaf (passes on a seeded break).
	FailsOnBreak *bool

	// Cost-surprise inputs.
	Duration  time.Duration // how long the check actually took
	Predicted time.Duration // a logged prediction of how long it should take; 0 = none

	// Correlated-provenance inputs.
	Deps   []string // identifiers this check's derivation depended on
	Agrees string   // the id of the check this one is claimed to corroborate
}

// Finding is a flagged check: which signal fired and the exact relation that fired
// it. Never a score, never a grade — a specific thing to look at.
type Finding struct {
	CheckID string
	Signal  Signal
	Detail  string
}

func (f Finding) String() string {
	return fmt.Sprintf("[%s] %s: %s", f.Signal, f.CheckID, f.Detail)
}

// boundaryTolerance is the relative closeness at which a result "equals" a cap.
// Small but non-zero so 20.5 vs 20.5 fires while 20.4 vs 20.5 does not.
const boundaryTolerance = 1e-9

// costSurpriseFactor: a check faster than this fraction of its prediction is flagged.
// Deliberately aggressive-but-not-tiny — the detector is weak and shouldn't cry wolf.
const costSurpriseFactor = 0.2

// BoundaryEqual fires when a measurement equals a known cap — a clamped value
// masquerading as a measurement. Needs both a result and a cap; without a declared
// cap there is nothing to compare and the detector stays silent.
func BoundaryEqual(c Check) *Finding {
	if c.Result == nil || c.Cap == nil {
		return nil
	}
	if nearlyEqual(*c.Result, *c.Cap) {
		return &Finding{
			CheckID: c.ID,
			Signal:  SignalBoundaryEqual,
			Detail: fmt.Sprintf("result %g%s equals the instrument cap %g%s — a clamped value, not a measurement",
				*c.Result, c.Unit, *c.Cap, c.Unit),
		}
	}
	return nil
}

// NonDiscriminating fires when a check provably does not fail on a seeded break
// (FailsOnBreak == false): it passes regardless of correctness, so it verifies
// nothing. This is the fig-leaf killer — a weak-but-technically-necessary falsifier
// (e.g. "the file was modified" for "the fix works") never fails a real mutation.
func NonDiscriminating(c Check) *Finding {
	if c.FailsOnBreak == nil || *c.FailsOnBreak {
		return nil
	}
	return &Finding{
		CheckID: c.ID,
		Signal:  SignalNonDiscriminating,
		Detail:  "check still passed when its target was deliberately broken — it cannot discriminate correct from incorrect",
	}
}

// CostSurprise fires when a check finished far cheaper than its own logged prediction.
// Requires a prediction; the weakest detector, and it says so in its own detail.
func CostSurprise(c Check) *Finding {
	if c.Predicted <= 0 || c.Duration <= 0 {
		return nil
	}
	if float64(c.Duration) < costSurpriseFactor*float64(c.Predicted) {
		return &Finding{
			CheckID: c.ID,
			Signal:  SignalCostSurprise,
			Detail: fmt.Sprintf("ran in %s against a predicted %s (%.0f%% of prediction) — implausibly cheap; may have short-circuited before checking",
				c.Duration, c.Predicted, 100*float64(c.Duration)/float64(c.Predicted)),
		}
	}
	return nil
}

// CorrelatedProvenance fires when two checks that CLAIM to corroborate each other
// share a dependency: their agreement is not independent evidence. Open limitation
// (see the design doc): in an LLM mesh the dangerous correlation is often shared
// training data or prompt bias, which is not a nameable dependency this can inspect.
func CorrelatedProvenance(a, b Check) *Finding {
	if a.Agrees != b.ID && b.Agrees != a.ID {
		return nil // they don't claim to corroborate each other
	}
	shared := intersect(a.Deps, b.Deps)
	if len(shared) == 0 {
		return nil
	}
	return &Finding{
		CheckID: a.ID,
		Signal:  SignalCorrelatedProvenance,
		Detail: fmt.Sprintf("agrees with %s but shares dependency %v — agreement is not independent evidence",
			b.ID, shared),
	}
}

// Capability is an exported entry point (command, message handler, tool) that ought
// to be reachable by a test through the real transport, not only a mock.
type Capability struct {
	Name              string
	ReachedByRealTest bool
}

// MissingBoundaryCoverage flags capabilities that no test reaches through the real
// path. This is the absence detector: the other four are negative signals over checks
// that EXIST, and a capability implemented, unit-tested in-process, and unreachable
// across its boundary passes all of them because there is nothing to flag.
func MissingBoundaryCoverage(caps []Capability) []Finding {
	var out []Finding
	for _, c := range caps {
		if !c.ReachedByRealTest {
			out = append(out, Finding{
				CheckID: c.Name,
				Signal:  SignalMissingBoundary,
				Detail:  "no test reaches this capability through the real transport — absence of a check, not a failing one",
			})
		}
	}
	return out
}

// Run applies the per-check detectors (1–4) to a set of checks and returns every
// finding. Deterministic order: checks as given, detectors in declaration order, then
// the pairwise correlated-provenance pass. It does NOT include boundary-coverage,
// which operates over capabilities, not checks — call MissingBoundaryCoverage for that.
func Run(checks []Check) []Finding {
	var out []Finding
	for _, c := range checks {
		if f := BoundaryEqual(c); f != nil {
			out = append(out, *f)
		}
		if f := NonDiscriminating(c); f != nil {
			out = append(out, *f)
		}
		if f := CostSurprise(c); f != nil {
			out = append(out, *f)
		}
	}
	for i := 0; i < len(checks); i++ {
		for j := i + 1; j < len(checks); j++ {
			if f := CorrelatedProvenance(checks[i], checks[j]); f != nil {
				out = append(out, *f)
			}
		}
	}
	return out
}

func nearlyEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	scale := math.Max(math.Abs(a), math.Abs(b))
	if scale == 0 {
		return diff < boundaryTolerance
	}
	return diff/scale < boundaryTolerance
}

func intersect(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, x := range a {
		set[x] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, y := range b {
		if set[y] && !seen[y] {
			out = append(out, y)
			seen[y] = true
		}
	}
	return out
}

// ptr helpers for building Checks concisely (tests, encoders).
func F(v float64) *float64 { return &v }
func B(v bool) *bool       { return &v }
