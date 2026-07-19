package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// twoNodeModel builds a positioned live model with an injected status cache (no
// store, no broker) so the animation path can be exercised deterministically.
func twoNodeModel(t *testing.T, bossStatus, workerStatus vitals.Status) Model {
	t.Helper()
	nodes := []store.Node{
		{NodeID: "1", Name: "boss", ParentID: "", CreatedAt: "2026-01-01T00:00:00Z"},
		{NodeID: "2", Name: "worker", ParentID: "1", CreatedAt: "2026-01-01T00:00:01Z"},
	}
	m := New(nodes)
	m.motion = motionCalm
	m.live = &liveState{
		statuses: map[string]vitals.Status{
			"boss":   bossStatus,
			"worker": workerStatus,
		},
		baseVersion: 1,
	}
	m.ensureBase()
	return m
}

// ledCell returns the [x,y] of a node's status LED (x+1, y — where drawCard puts
// it), found by name in the positioned tree (m.flat carries the located nodes).
func ledCell(t *testing.T, m Model, name string) [2]int {
	t.Helper()
	for _, n := range m.flat {
		if n.ID == name {
			return [2]int{n.X + 1, n.Y}
		}
	}
	t.Fatalf("node %q not found in flattened tree", name)
	return [2]int{}
}

func TestLEDBreathesThroughDistinctLevels(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true
	defer func(i int) { currentTheme = i }(currentTheme)
	currentTheme = 0

	m := twoNodeModel(t, vitals.StatusActive, vitals.StatusQuiet)
	m.cursor = -1 // deselect, so the selection highlight doesn't override the LED
	boss := ledCell(t, m, "boss")

	// Pulse(0,16)=0 → faint (SGR 2); Pulse(4,16)=0.5 → normal; Pulse(8,16)=1 → bold (SGR 1).
	m.frame = 0
	ov0 := m.buildOverlay()
	m.frame = 4
	ov4 := m.buildOverlay()
	m.frame = 8
	ov8 := m.buildOverlay()

	c0, ok0 := ov0[boss]
	c4, ok4 := ov4[boss]
	c8, ok8 := ov8[boss]
	if !ok0 || !ok4 || !ok8 {
		t.Fatalf("active node's LED must be in the overlay every frame; got %v %v %v", ok0, ok4, ok8)
	}
	if !strings.Contains(c0.ansi, "\x1b[2;") {
		t.Errorf("frame 0 (trough) should be faint (SGR 2), got %q", c0.ansi)
	}
	if strings.Contains(c4.ansi, "\x1b[2;") || strings.Contains(c4.ansi, "\x1b[1;") {
		t.Errorf("frame 4 (mid) should be normal intensity, got %q", c4.ansi)
	}
	if !strings.Contains(c8.ansi, "\x1b[1;") {
		t.Errorf("frame 8 (peak) should be bold (SGR 1), got %q", c8.ansi)
	}
	if c0.ansi == c8.ansi {
		t.Errorf("trough and peak must differ — the LED is not breathing")
	}
	// The overlay only ever repaints; it never swaps the ● rune.
	if c0.r != 0 {
		t.Errorf("LED overlay must keep the base rune (r==0), got %q", c0.r)
	}
}

func TestQuietNodeDoesNotBreathe(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true

	m := twoNodeModel(t, vitals.StatusActive, vitals.StatusQuiet)
	worker := ledCell(t, m, "worker")
	m.frame = 8
	if _, ok := m.buildOverlay()[worker]; ok {
		t.Errorf("a quiet node must never appear in the animation overlay — pulse is active-only")
	}
}

func TestLivelyBreathesWholeBorderCalmDoesNot(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true

	m := twoNodeModel(t, vitals.StatusActive, vitals.StatusQuiet)
	m.cursor = -1 // deselect, so the selection highlight doesn't paint the border
	boss := ledCell(t, m, "boss") // (x+1, y) — a top-border cell
	// A left-edge border cell that is NOT the LED: (x, y).
	edge := [2]int{boss[0] - 1, boss[1]}
	m.frame = 3 // mid-breath so levels are non-trivial

	m.motion = motionCalm
	if _, ok := m.buildOverlay()[edge]; ok {
		t.Errorf("calm mode must only breathe the LED, not the whole border")
	}
	m.motion = motionLively
	if _, ok := m.buildOverlay()[edge]; !ok {
		t.Errorf("lively mode must breathe the full active-card border")
	}
}

func TestSelectionHighlightsSelectedCardBorder(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true

	// Both quiet (no breath), so any border overlay is the selection highlight.
	m := twoNodeModel(t, vitals.StatusQuiet, vitals.StatusQuiet)
	m.frame = 5
	m.cursor = 0 // select the root ("boss")

	bossEdge := [2]int{ledCell(t, m, "boss")[0] - 1, ledCell(t, m, "boss")[1]} // a border cell
	workerEdge := [2]int{ledCell(t, m, "worker")[0] - 1, ledCell(t, m, "worker")[1]}

	ov := m.buildOverlay()
	if c, ok := ov[bossEdge]; !ok || !strings.Contains(c.ansi, "\x1b[1;") {
		t.Errorf("selected card's border should be bold-highlighted, got %v (ok=%v)", c, ok)
	}
	if _, ok := ov[workerEdge]; ok {
		t.Errorf("a non-selected, quiet card must not be highlighted")
	}
}

func TestMotionOffProducesNoOverlay(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true

	m := twoNodeModel(t, vitals.StatusActive, vitals.StatusActive)
	m.motion = motionOff
	m.frame = 8
	if len(m.buildOverlay()) != 0 {
		t.Errorf("motion off must yield an empty overlay")
	}
}

func TestStillFrameMatchesStyledRenderer(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true
	defer func(i int) { currentTheme = i }(currentTheme)
	currentTheme = 0

	// No active nodes AND no selection → empty overlay every frame → the animated
	// path must be byte-identical to the plain styled renderer.
	m := twoNodeModel(t, vitals.StatusQuiet, vitals.StatusQuiet)
	m.cursor = -1
	m.frame = 8
	animated := m.renderAnimated()
	styled := RenderStyled(m.root, m.width, func(id string) vitals.Status {
		return m.live.statuses[id]
	})
	if animated != styled {
		t.Errorf("a still frame must equal RenderStyled byte-for-byte\nanimated:\n%q\nstyled:\n%q", animated, styled)
	}
}
