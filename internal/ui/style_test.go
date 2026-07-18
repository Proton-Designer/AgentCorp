package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

func twoNodeTree() *layout.Node {
	child := &layout.Node{ID: "worker", W: cardW, H: cardH}
	return &layout.Node{ID: "ceo", W: cardW, H: cardH, Children: []*layout.Node{child}}
}

// The plain renderer must stay ANSI-free — the layout tests and any width count
// depend on it being a true cell grid.
func TestPlainRenderHasNoANSI(t *testing.T) {
	out := Render(twoNodeTree(), 80)
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain Render leaked ANSI escapes:\n%q", out)
	}
}

// The styled renderer colours each card by status: an active node and a dead
// node must carry different foreground escapes.
func TestStyledRenderColorsByStatus(t *testing.T) {
	old := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = old }()

	status := map[string]vitals.Status{"ceo": vitals.StatusActive, "worker": vitals.StatusDead}
	out := RenderStyled(twoNodeTree(), 80, func(id string) vitals.Status { return status[id] })

	if !strings.Contains(out, "\x1b[") {
		t.Fatal("styled render emitted no ANSI at all")
	}
	if !strings.Contains(out, ansiFor(styActive)) {
		t.Fatal("active node's colour missing")
	}
	if !strings.Contains(out, ansiFor(styDead)) {
		t.Fatal("dead node's colour missing")
	}
	if !strings.Contains(out, ansiFor(styConnector)) {
		t.Fatal("connector colour missing")
	}
	// Every opened colour run must be reset, or colour bleeds past the chart.
	if strings.Count(out, ansiReset) == 0 {
		t.Fatal("no ANSI resets emitted — colour would bleed")
	}
}

// Stripping the ANSI from a styled render must reproduce the plain render
// exactly: colour is a pure overlay, never a geometry change.
func TestStyledStripsToPlainGeometry(t *testing.T) {
	old := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = old }()

	tree := twoNodeTree()
	plain := Render(tree, 80)
	tree2 := twoNodeTree()
	styled := RenderStyled(tree2, 80, func(string) vitals.Status { return vitals.StatusQuiet })

	if stripANSI(styled) != plain {
		t.Fatalf("styled geometry diverged from plain:\nplain:\n%q\nstripped:\n%q", plain, stripANSI(styled))
	}
}

// With colour disabled, the styled renderer must equal the plain one byte for
// byte (NO_COLOR / non-colour terminals).
func TestStyledFallsBackWhenColorDisabled(t *testing.T) {
	old := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = old }()

	tree := twoNodeTree()
	if got := RenderStyled(tree, 80, func(string) vitals.Status { return vitals.StatusActive }); got != Render(twoNodeTree(), 80) {
		t.Fatal("with colour disabled, styled render must equal plain render")
	}
}

// stripANSI removes SGR escape sequences so styled output can be compared to
// plain geometry.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
