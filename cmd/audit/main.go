// Command audit runs the epistemic auditor (internal/audit) over a codebase.
//
// It is a standalone tool, deliberately not folded into the agentcorp binary, so
// the auditor can be pointed at ANY repository — including foreign ones in other
// languages — not just AgentCorp itself.
//
// Modes:
//
//	audit invariants --lang go|ts --dir PATH
//	    Flag declarations whose attached comment asserts a guarantee that no test
//	    references by symbol — a documented invariant with nothing checking it.
//	audit probe --dir PATH --pkg internal/foo
//	    Mutate boolean returns in a Go package and report mutants no test catches.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/audit"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "invariants":
		runInvariants(os.Args[2:])
	case "probe":
		runProbe(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: audit invariants --lang go|ts --dir PATH")
	fmt.Fprintln(os.Stderr, "       audit probe --dir PATH --pkg internal/foo [--max N]")
	os.Exit(2)
}

func runInvariants(args []string) {
	fs := flag.NewFlagSet("invariants", flag.ExitOnError)
	lang := fs.String("lang", "go", "go|ts")
	dir := fs.String("dir", ".", "root directory to scan")
	coverage := fs.Bool("coverage", false, "Go only: filter to symbols the test suite does not reach (higher precision)")
	_ = fs.Parse(args)

	var l audit.Lang
	var srcExt string
	var isTest func(string) bool
	switch *lang {
	case "go":
		l, srcExt = audit.LangGo, ".go"
		isTest = func(p string) bool { return strings.HasSuffix(p, "_test.go") }
	case "ts":
		l, srcExt = audit.LangTS, ".ts"
		isTest = func(p string) bool {
			return strings.HasSuffix(p, ".test.ts") || strings.HasSuffix(p, ".spec.ts") ||
				strings.Contains(p, "__tests__")
		}
	default:
		usage()
	}

	src, tests := collect(*dir, srcExt, isTest)
	findings := audit.ScanInvariants(l, src, tests)

	dropped := 0
	if *coverage {
		if l != audit.LangGo {
			fmt.Fprintln(os.Stderr, "--coverage is Go-only (needs the go test/cover toolchain)")
			os.Exit(2)
		}
		cov, err := audit.FuncCoverage(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "coverage:", err)
			os.Exit(1)
		}
		var kept, drop []audit.InvariantFinding
		kept, drop = audit.FilterUnreached(findings, cov)
		findings, dropped = kept, len(drop)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})

	fmt.Printf("# invariant-comment audit — %s (%s)\n", *dir, *lang)
	if *coverage {
		fmt.Printf("# %d source files, %d test files, %d findings (%d dropped as reached-by-tests)\n", len(src), len(tests), len(findings), dropped)
	} else {
		fmt.Printf("# %d source files, %d test files, %d raw findings\n", len(src), len(tests), len(findings))
	}
	fmt.Println("# RAW findings for cold grading — a documented guarantee whose symbol no test names.")
	fmt.Println("# Known limit: invariant-language grep conflates CONTRACT with RATIONALE; expect false positives on discursive comments.")
	for _, f := range findings {
		fmt.Printf("\n%s:%d  %s  [keyword: %q]\n    %s\n", f.File, f.Line, f.Symbol, f.Keyword, f.Snippet)
	}
}

func runProbe(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	dir := fs.String("dir", ".", "module root")
	pkg := fs.String("pkg", "", "package path relative to module root, e.g. internal/vitals")
	max := fs.Int("max", 40, "max mutants")
	fs.Parse(args)
	if *pkg == "" {
		usage()
	}
	rep, err := audit.ProbeDiscrimination(*dir, *pkg, *max, 90*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
	fmt.Printf("# discrimination probe — %s: %d mutants, %d survived (fig leaves)\n", rep.Package, len(rep.Mutants), len(rep.Survived))
	for _, m := range rep.Mutants {
		fmt.Printf("%s:%d %s  %q->%q  [%s]\n", m.File, m.Line, m.Function, m.Original, m.Mutated, m.Outcome)
	}
}

// collect walks dir and returns source (non-test) and test file contents by path.
func collect(dir, ext string, isTest func(string) bool) (src, tests map[string]string) {
	src, tests = map[string]string{}, map[string]string{}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == "node_modules" || base == ".git" || base == "scratch" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ext) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if isTest(path) {
			tests[rel] = string(b)
		} else {
			src[rel] = string(b)
		}
		return nil
	})
	return src, tests
}
