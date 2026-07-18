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

// Styled render adds colour AND a per-card status glyph over identical RUNE
// positions: strip the ANSI, map the status LED back to a border dash, and the
// result must reproduce the plain render exactly. That proves the styled path
// only overlays and never moves a rune.
//
// LOAD-BEARING LIMITATION (flagged in review by the terminal-MCP team): this
// verifies rune POSITION, NOT terminal DISPLAY WIDTH. The normalization
// deliberately erases ● ○ × ◌ before comparing — which are the exact glyphs
// whose width is in question. ● ○ × are East-Asian-Ambiguous (UCD): a terminal
// may render them at 1 OR 2 cells, and this grid assumes 1 (rune count). If a
// terminal commits 2, the whole card border shifts — and NO Go unit test can
// catch that, because Go cannot know a terminal's display width. Confirming it
// requires driving a real terminal (precisely the gap the terminal-automation
// MCP exists to close). So: this test proves the overlay is positionally clean;
// it CANNOT and does not certify the render is visually aligned. See UI-7.
func TestStyledOverlaysWithoutMovingRunes(t *testing.T) {
	old := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = old }()

	plain := Render(twoNodeTree(), 80)
	styled := RenderStyled(twoNodeTree(), 80, func(string) vitals.Status { return vitals.StatusQuiet })

	// Map the status LED back to a border dash. NOTE: this erases the very
	// glyphs whose display width is unverifiable here — a rune-position check,
	// not a display-width check.
	normalized := strings.NewReplacer("○", "─", "●", "─", "×", "─", "◌", "─").Replace(stripANSI(styled))
	if normalized != plain {
		t.Fatalf("styled rune-geometry diverged from plain:\nplain:\n%q\nnormalized:\n%q", plain, normalized)
	}
}

// statusGlyphWidthRisk pins the honest fact that the fixed-grid status LEDs are
// an unverified display-width risk, so the assumption is loud in the suite
// rather than buried. ◌ is spec-Neutral (safe control); ● ○ × are
// East-Asian-Ambiguous and can render 2 cells wide on some terminals — which no
// unit test here can detect.
func TestStatusGlyphsCarryDocumentedWidthRisk(t *testing.T) {
	ambiguous := map[rune]bool{'●': true, '○': true, '×': true}
	for _, st := range []vitals.Status{vitals.StatusActive, vitals.StatusQuiet, vitals.StatusDead, vitals.StatusPending} {
		g := statusGlyph(st)
		if g == 0 {
			t.Fatalf("status %q lost its glyph", st)
		}
		// ◌ (pending) is Neutral and safe; the rest are ambiguous-width risks.
		// This assertion exists to fail loudly if someone swaps in a new glyph
		// without re-checking its width class.
		if g != '◌' && !ambiguous[g] {
			t.Fatalf("status glyph %q for %q is neither the safe control ◌ nor a known ambiguous LED — re-verify its display-width class before shipping it in a fixed grid", string(g), st)
		}
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

// Each card carries a status LED glyph in its top border in the styled path.
func TestStyledRenderHasStatusGlyph(t *testing.T) {
	old := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = old }()

	status := map[string]vitals.Status{"ceo": vitals.StatusActive, "worker": vitals.StatusDead}
	out := RenderStyled(twoNodeTree(), 80, func(id string) vitals.Status { return status[id] })
	if !strings.Contains(out, "●") {
		t.Fatal("active card missing its ● status LED")
	}
	if !strings.Contains(out, "×") {
		t.Fatal("dead card missing its × status LED")
	}
}
