package audit

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Coverage-reachability refinement (Go only). The invariant-comment detector's
// name-matching false positives, found by the cross-team blind grade, all had the
// same cause: well-factored code tests an internal symbol THROUGH its public entry
// point, so no test NAMES the symbol even though the code is thoroughly exercised.
// Symbol-reference is the wrong proxy; REACHABILITY is the right one. This computes
// which functions the test suite actually executes (via go's own coverage), so an
// invariant-comment on a function tests never reach is a far stronger finding than
// one merely not mentioned by name.
//
// Honest limit, carried from metamorphic testing: coverage is EXECUTED, not
// DISCRIMINATED — a covered function may be run by a test that asserts nothing about
// it. So zero coverage is high-confidence "unchecked"; non-zero coverage is only
// "reached," not "verified." The refinement removes the name-matching false
// positives; it does not promise the covered remainder is sound.

// coverFuncLine parses one row of `go tool cover -func`:
//
//	internal/store/nodes.go:194:  SetState    0.0%
var coverFuncLine = regexp.MustCompile(`^(\S+\.go):\d+:\s+(\S+)\s+([\d.]+)%$`)

// FuncCoverage runs the module's test suite with coverage and returns, per function
// name, its statement coverage percent (0..100). A function absent from the map was
// not compiled into any covered package. Best-effort: a failing suite still yields
// whatever coverage was produced.
func FuncCoverage(moduleRoot string) (map[string]float64, error) {
	profile := filepath.Join(os.TempDir(), "audit-cover.out")
	defer os.Remove(profile)

	// -coverpkg=./... so internal packages are instrumented even when exercised
	// only by another package's tests — exactly the through-the-public-entry-point
	// case this refinement exists to see.
	test := exec.Command("go", "test", "-coverpkg=./...", "-coverprofile="+profile, "./...")
	test.Dir = moduleRoot
	_ = test.Run() // a red test still leaves a usable profile; don't abort on it

	fn := exec.Command("go", "tool", "cover", "-func="+profile)
	fn.Dir = moduleRoot
	out, err := fn.Output()
	if err != nil {
		return nil, err
	}
	cov := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		m := coverFuncLine.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		pct, _ := strconv.ParseFloat(m[3], 64)
		// Keep the max seen for a name (methods on different types can share a
		// short name; the reachability question is "is ANY reached").
		if prev, ok := cov[m[2]]; !ok || pct > prev {
			cov[m[2]] = pct
		}
	}
	return cov, nil
}

// FilterUnreached keeps only invariant findings whose symbol the test suite does
// NOT execute (zero coverage, or absent from the profile). This is the high-
// confidence subset: a documented guarantee on code no test even runs. Findings on
// reached symbols are dropped as the name-matching false positives they are.
func FilterUnreached(findings []InvariantFinding, cov map[string]float64) (kept, dropped []InvariantFinding) {
	for _, f := range findings {
		if pct, ok := cov[f.Symbol]; ok && pct > 0 {
			dropped = append(dropped, f)
		} else {
			kept = append(kept, f)
		}
	}
	return kept, dropped
}
