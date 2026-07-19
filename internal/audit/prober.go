package audit

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The discrimination prober is the PROSPECTIVE half of the auditor (the fig-leaf
// killer, detector #2 run over live code). It applies a small, safe mutation to a
// function — negating a boolean return, which still compiles but changes behaviour —
// re-runs that package's tests, and asks the only question that matters: did any
// test FAIL? A mutant that SURVIVES (tests still pass) proves the tests do not
// discriminate that behaviour: they pass regardless of correctness. This is
// verification-by-reproduction pointed at the verifications themselves.
//
// Safety: it never mutates the working tree. It copies the module's source into a
// temp directory and mutates the COPY, so an interrupted run cannot corrupt the repo.

// MutantResult records one mutation and what the tests did with it.
type MutantResult struct {
	File     string // repo-relative
	Line     int
	Function string
	Original string // the return expression before mutation
	Mutated  string // after
	Outcome  string // "caught" (tests failed — good) | "survived" (fig leaf) | "invalid" (didn't compile)
}

// ProbeReport is the outcome of probing one package.
type ProbeReport struct {
	Package  string
	Mutants  []MutantResult
	Survived []MutantResult // the findings: mutations no test caught
}

// ProbeDiscrimination mutates boolean returns in the non-test .go files of pkgRel
// (a module-relative path like "internal/vitals"), runs that package's tests against
// each mutant in an isolated copy of the module, and reports which mutants survived.
// maxMutants caps the work; perTestTimeout bounds each test run.
func ProbeDiscrimination(moduleRoot, pkgRel string, maxMutants int, perTestTimeout time.Duration) (*ProbeReport, error) {
	tmp, err := os.MkdirTemp("", "audit-probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := copyModuleSource(moduleRoot, tmp); err != nil {
		return nil, fmt.Errorf("copy module: %w", err)
	}

	pkgDir := filepath.Join(moduleRoot, pkgRel)
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, err
	}

	rep := &ProbeReport{Package: pkgRel}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		rel := filepath.Join(pkgRel, name)
		muts, err := boolReturnMutants(filepath.Join(moduleRoot, rel))
		if err != nil {
			continue // unparseable file: skip, don't abort the run
		}
		for _, m := range muts {
			if len(rep.Mutants) >= maxMutants {
				return finalize(rep), nil
			}
			m.File = rel
			outcome := runMutant(tmp, pkgRel, rel, m, perTestTimeout)
			m.Outcome = outcome
			rep.Mutants = append(rep.Mutants, m)
		}
	}
	return finalize(rep), nil
}

func finalize(rep *ProbeReport) *ProbeReport {
	for _, m := range rep.Mutants {
		if m.Outcome == "survived" {
			rep.Survived = append(rep.Survived, m)
		}
	}
	return rep
}

// boolReturnMutants finds negatable boolean returns in a file and returns one
// candidate mutation per site (as source text, plus location).
func boolReturnMutants(path string) ([]MutantResult, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var out []MutantResult
	var curFn string
	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			curFn = fn.Name.Name
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		orig, mut, ok := negateBool(ret.Results[0])
		if !ok {
			return true
		}
		out = append(out, MutantResult{
			Line:     fset.Position(ret.Pos()).Line,
			Function: curFn,
			Original: orig,
			Mutated:  mut,
		})
		return true
	})
	return out, nil
}

// negateBool returns (originalText, mutatedText, ok) for boolean-ish return
// expressions that can be safely negated in a way that still compiles.
func negateBool(e ast.Expr) (string, string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "true" {
			return "true", "false", true
		}
		if x.Name == "false" {
			return "false", "true", true
		}
	case *ast.UnaryExpr:
		if x.Op == token.NOT {
			return "!" + exprText(x.X), exprText(x.X), true // drop the !
		}
	case *ast.BinaryExpr:
		// Comparisons flip to their negation; logical &&/|| flip to each other
		// (a && b vs a || b differ on real inputs, and both compile). These are the
		// common shapes of a boolean return.
		flip := map[token.Token]token.Token{
			token.EQL: token.NEQ, token.NEQ: token.EQL,
			token.LSS: token.GEQ, token.GEQ: token.LSS,
			token.GTR: token.LEQ, token.LEQ: token.GTR,
			token.LAND: token.LOR, token.LOR: token.LAND,
		}
		if to, ok := flip[x.Op]; ok {
			orig := exprText(x.X) + " " + x.Op.String() + " " + exprText(x.Y)
			mut := exprText(x.X) + " " + to.String() + " " + exprText(x.Y)
			return orig, mut, true
		}
	}
	return "", "", false
}

func exprText(e ast.Expr) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, token.NewFileSet(), e)
	return b.String()
}

// runMutant applies one mutation to the copied file, runs the package tests, and
// classifies the outcome, then restores the copied file. Never touches moduleRoot.
func runMutant(tmpRoot, pkgRel, fileRel string, m MutantResult, timeout time.Duration) string {
	target := filepath.Join(tmpRoot, fileRel)
	orig, err := os.ReadFile(target)
	if err != nil {
		return "invalid"
	}
	mutated, ok := applyLineMutation(orig, m)
	if !ok {
		return "invalid"
	}
	if err := os.WriteFile(target, mutated, 0o644); err != nil {
		return "invalid"
	}
	defer os.WriteFile(target, orig, 0o644) // always restore the copy

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./"+pkgRel+"/")
	cmd.Dir = tmpRoot
	outBytes, _ := cmd.CombinedOutput()
	out := string(outBytes)

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return "invalid" // timed out — inconclusive, don't count as a survivor
	case strings.Contains(out, "[build failed]") || strings.Contains(out, "cannot use") ||
		strings.Contains(out, "undefined:") || strings.Contains(out, "syntax error"):
		return "invalid" // the mutation didn't compile — not a real mutant
	case strings.Contains(out, "\nFAIL") || strings.HasPrefix(out, "FAIL") || strings.Contains(out, "--- FAIL"):
		return "caught" // a test failed — the suite discriminated the mutation
	case strings.Contains(out, "ok  ") || strings.Contains(out, "no test files"):
		return "survived" // tests passed despite the broken behaviour — a fig leaf
	default:
		return "invalid"
	}
}

// applyLineMutation replaces the first occurrence of the original return expression
// on the recorded line with the mutated text. Line-scoped so it can't accidentally
// rewrite an identical expression elsewhere in the file.
func applyLineMutation(src []byte, m MutantResult) ([]byte, bool) {
	lines := strings.Split(string(src), "\n")
	if m.Line < 1 || m.Line > len(lines) {
		return nil, false
	}
	i := m.Line - 1
	if !strings.Contains(lines[i], m.Original) {
		return nil, false
	}
	lines[i] = strings.Replace(lines[i], m.Original, m.Mutated, 1)
	return []byte(strings.Join(lines, "\n")), true
}

// copyModuleSource copies the Go module's source (go.mod, go.sum, and the internal/
// and cmd/ trees) into dst so the prober can mutate and test in isolation.
func copyModuleSource(root, dst string) error {
	for _, f := range []string{"go.mod", "go.sum"} {
		if err := copyFile(filepath.Join(root, f), filepath.Join(dst, f)); err != nil {
			return err
		}
	}
	for _, dir := range []string{"internal", "cmd"} {
		if err := copyTree(filepath.Join(root, dir), filepath.Join(dst, dir)); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
