// Package company scopes a company of agents to a directory subtree.
//
// A company is a stable {id, name} pair recorded in a `.agentcorp/company.toml`
// file at the root of a directory tree. Resolution walks up from a starting
// directory to the nearest such file, exactly like git finding `.git` — the
// nearest company wins, so a nested subtree can carry its own company without
// leaking into its parent's. Everything an operator sees is then filtered to
// peers whose working directory resolves to the same company, so one laptop
// running many unrelated Claude sessions no longer shows them all in one chart.
//
// config.go is the pure, I/O-free core: parse a config file's bytes into a
// Company and format a Company back to bytes. Keeping it pure is deliberate —
// this is the parsing surface most likely to mishandle a hostile or hand-edited
// file, so it is exhaustively table-testable without touching a filesystem.
package company

import (
	"fmt"
	"sort"
	"strings"
)

// ConfigName is the directory and file a company definition lives in,
// relative to the subtree root: <root>/.agentcorp/company.toml.
const (
	ConfigDir  = ".agentcorp"
	ConfigFile = "company.toml"
)

// Company is the identity bound to a directory subtree.
//
// ID is generated once at creation and must never change — it is what lets two
// sessions in the same subtree recognize each other as the same company even if
// the human-facing Name is later edited. Name is a free label for display.
type Company struct {
	ID   string
	Name string
}

// ParseConfig reads a company definition from raw config bytes.
//
// It accepts a deliberately small subset of TOML: blank lines, `#` comments,
// and `key = value` pairs where the value is either a double-quoted basic
// string (with \\, \", \n, \t escapes) or a bare token taken literally. Only
// `id` and `name` are recognized; any other key is ignored so a future field
// never makes an older AgentCorp reject the file. Both id and name are
// required and must be non-empty after trimming.
//
// A malformed file is an error, never a silently-empty Company — a company
// whose identity we cannot read is a problem an operator must see, not one to
// paper over with a blank name.
func ParseConfig(data []byte) (Company, error) {
	var c Company
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return Company{}, fmt.Errorf("company config line %d: not a key = value pair: %q", i+1, raw)
		}
		key := strings.TrimSpace(line[:eq])
		val, err := unquote(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			return Company{}, fmt.Errorf("company config line %d (%s): %w", i+1, key, err)
		}
		switch key {
		case "id":
			c.ID = val
		case "name":
			c.Name = val
		default:
			// Unknown key: ignore for forward compatibility.
		}
	}
	if c.ID == "" {
		return Company{}, fmt.Errorf("company config: missing or empty 'id'")
	}
	if c.Name == "" {
		return Company{}, fmt.Errorf("company config: missing or empty 'name'")
	}
	return c, nil
}

// FormatConfig renders a Company as the bytes of a company.toml file. It is the
// inverse of ParseConfig: FormatConfig then ParseConfig round-trips any Company
// with a non-empty id and name, including names containing quotes or newlines.
func FormatConfig(c Company) []byte {
	var b strings.Builder
	b.WriteString("# AgentCorp company definition.\n")
	b.WriteString("# Binds this directory subtree to a company. Managed by AgentCorp:\n")
	b.WriteString("# edit 'name' freely, but leave 'id' stable — it is how sessions in\n")
	b.WriteString("# this tree recognize each other as the same company.\n")
	b.WriteString("id = " + quote(c.ID) + "\n")
	b.WriteString("name = " + quote(c.Name) + "\n")
	return []byte(b.String())
}

// quote wraps s as a TOML basic string, escaping the characters that would
// otherwise break the single-line `key = "value"` shape.
func quote(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\t", `\t`,
	)
	return `"` + r.Replace(s) + `"`
}

// unquote reverses quote for a double-quoted value, or returns a bare token
// unchanged. A value that opens a quote but never closes it is an error rather
// than a truncated string.
func unquote(v string) (string, error) {
	if !strings.HasPrefix(v, `"`) {
		return v, nil
	}
	body := v[1:]
	var out strings.Builder
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if ch == '"' {
			// Closing quote: anything after it is stray but harmless (a trailing
			// comment would already have been left intact; we simply stop here).
			return out.String(), nil
		}
		if ch != '\\' {
			out.WriteByte(ch)
			continue
		}
		i++
		if i >= len(body) {
			return "", fmt.Errorf("dangling escape at end of quoted value")
		}
		switch body[i] {
		case '\\':
			out.WriteByte('\\')
		case '"':
			out.WriteByte('"')
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		default:
			return "", fmt.Errorf("unknown escape \\%c in quoted value", body[i])
		}
	}
	return "", fmt.Errorf("unterminated quoted value: %q", v)
}

// keysOf is a tiny helper used by tests to assert recognized keys stay stable;
// kept here so the recognized set has a single source of truth.
func keysOf() []string {
	ks := []string{"id", "name"}
	sort.Strings(ks)
	return ks
}
