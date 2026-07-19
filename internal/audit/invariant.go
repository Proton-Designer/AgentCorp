package audit

import (
	"regexp"
	"strings"
)

// The invariant-comment detector (proposed by the terminal-MCP lead during design).
// A doc comment that ASSERTS a guarantee is a claim; a documented invariant with no
// test naming the symbol it guards is an UNATTESTED claim — worse than an untested
// function, because the comment manufactures confidence in every reader. This is the
// one detector that reaches the class the artifact-auditor otherwise can't: a claim
// with no artifact — except here the comment IS the artifact. It is grep + symbol
// resolution, so unlike the mutation prober it is LANGUAGE-AGNOSTIC.
//
// Known limitation, pre-registered before the first cross-codebase run: invariant-
// language grep conflates CONTRACTS ("refuses X", on a function) with RATIONALE
// ("silently looking correct is this project's signature failure", exposition). We
// mitigate by only inspecting a comment block CONTIGUOUS with a declaration —
// contracts sit on the thing they constrain — but a discursive codebase will still
// produce false positives, and the raw findings are meant to be graded, not trusted.

// Lang selects declaration syntax.
type Lang int

const (
	LangGo Lang = iota
	LangTS
)

// InvariantFinding is a declaration whose attached comment asserts a guarantee that
// no test references by symbol.
type InvariantFinding struct {
	File    string
	Line    int
	Symbol  string
	Keyword string
	Snippet string
}

// invariantKeywords is contract/guarantee language. Deliberately inclusive: the
// output is raw findings for cold grading, and a high false-positive rate on a
// discursive codebase is itself a reportable result about the detector.
var invariantKeywords = []string{
	"never", "always", "must not", "must ", "refuses", "refuse", "rejects", "reject",
	"guarantee", "cannot", "can not", "exactly one", "will not", "invariant",
	"ensures", "forbids", "no caller", "not allowed", "only ever", "never silently",
}

var declRegexes = map[Lang][]*regexp.Regexp{
	LangGo: {
		regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?([A-Za-z_]\w*)`),
		regexp.MustCompile(`^type\s+([A-Za-z_]\w*)`),
		regexp.MustCompile(`^(?:var|const)\s+([A-Za-z_]\w*)`),
	},
	LangTS: {
		regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$]\w*)`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_$]\w*)`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$]\w*)`),
		regexp.MustCompile(`^\s*(?:public|private|protected|readonly|static|async|get|set)?\s*([A-Za-z_$]\w*)\s*\(`),
	},
}

// ScanInvariants finds invariant-language comments attached to a declaration whose
// symbol appears in NO test file. sourceFiles and testFiles map path→content;
// testFiles supply the symbol set that counts as "attested".
func ScanInvariants(lang Lang, sourceFiles, testFiles map[string]string) []InvariantFinding {
	tested := testSymbolSet(testFiles)
	var out []InvariantFinding
	for path, content := range sourceFiles {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			sym, ok := matchDecl(lang, line)
			if !ok {
				continue
			}
			block, blockStart := commentBlockAbove(lines, i)
			if block == "" {
				continue
			}
			kw, has := hasInvariantKeyword(block)
			if !has {
				continue
			}
			if tested[sym] {
				continue // some test names the symbol — attested (necessary, not sufficient)
			}
			out = append(out, InvariantFinding{
				File: path, Line: blockStart + 1, Symbol: sym, Keyword: kw,
				Snippet: snippet(block),
			})
		}
	}
	return out
}

func matchDecl(lang Lang, line string) (string, bool) {
	for _, re := range declRegexes[lang] {
		if m := re.FindStringSubmatch(line); m != nil {
			// filter language keywords that the loose method regex can catch
			if isNoise(m[1]) {
				return "", false
			}
			return m[1], true
		}
	}
	return "", false
}

var tsNoise = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"return": true, "function": true, "constructor": true, "super": true,
}

func isNoise(sym string) bool { return tsNoise[sym] }

// commentBlockAbove returns the contiguous comment block immediately above line i
// (no blank line between it and the declaration) and its starting line index.
func commentBlockAbove(lines []string, i int) (string, int) {
	j := i - 1
	end := j
	for j >= 0 {
		t := strings.TrimSpace(lines[j])
		if isCommentLine(t) {
			j--
			continue
		}
		break
	}
	start := j + 1
	if start > end {
		return "", 0
	}
	return strings.Join(lines[start:end+1], "\n"), start
}

func isCommentLine(t string) bool {
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") ||
		strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*/")
}

func hasInvariantKeyword(block string) (string, bool) {
	low := strings.ToLower(block)
	for _, kw := range invariantKeywords {
		if strings.Contains(low, kw) {
			return strings.TrimSpace(kw), true
		}
	}
	return "", false
}

var wordRe = regexp.MustCompile(`[A-Za-z_$]\w*`)

func testSymbolSet(testFiles map[string]string) map[string]bool {
	set := map[string]bool{}
	for _, content := range testFiles {
		for _, w := range wordRe.FindAllString(content, -1) {
			set[w] = true
		}
	}
	return set
}

func snippet(block string) string {
	for _, ln := range strings.Split(block, "\n") {
		t := strings.TrimLeft(strings.TrimSpace(ln), "/* ")
		if t != "" {
			if len(t) > 100 {
				t = t[:100] + "…"
			}
			return t
		}
	}
	return ""
}
