package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

func TestClickMapsToTheRightCard(t *testing.T) {
	m := twoNodeModel(t, vitals.StatusQuiet, vitals.StatusQuiet)
	m.view = viewTree
	m.recordNodeRects(3) // tree: chart starts 3 rows down (header, hud, blank)

	rectOf := func(name string) (nodeRect, bool) {
		for _, r := range m.live.nodeRects {
			if r.name == name {
				return r, true
			}
		}
		return nodeRect{}, false
	}

	boss, ok := rectOf("boss")
	if !ok {
		t.Fatal("boss card was not recorded")
	}
	worker, ok := rectOf("worker")
	if !ok {
		t.Fatal("worker card was not recorded")
	}

	// A click inside the boss card resolves to boss; inside worker resolves to worker.
	if name, hit := m.nodeAt(boss.x+1, boss.y+1); !hit || name != "boss" {
		t.Errorf("click inside boss card should hit boss, got %q hit=%v", name, hit)
	}
	if name, hit := m.nodeAt(worker.x+2, worker.y+1); !hit || name != "worker" {
		t.Errorf("click inside worker card should hit worker, got %q hit=%v", name, hit)
	}
	// Empty space hits nothing.
	if _, hit := m.nodeAt(0, 0); hit {
		t.Errorf("click on empty space must hit no card")
	}
	// The recorded card carries the centered prefix — it isn't at column 0.
	if boss.x == 0 {
		t.Errorf("card x should include the chart's centering offset, got 0")
	}
	if boss.y != 3 {
		t.Errorf("root card should sit on the chart's first row (baseRow 3), got y=%d", boss.y)
	}
}

// TestClickRectMatchesRender is the real correctness check: it renders the actual
// View and confirms each recorded card rectangle's top-left corner lands exactly on
// a "╭" in the rendered output. If the offset math drifts from the renderer, this
// fails — so a click can't silently map to the wrong cell.
func TestClickRectMatchesRender(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false // plain text so we can scan for the corner glyph

	for _, view := range []viewMode{viewTree, viewMission} {
		m := twoNodeModel(t, vitals.StatusQuiet, vitals.StatusQuiet)
		m.width, m.height, m.motion, m.view = 100, 30, motionOff, view

		out := m.View() // View() records the rects as it renders
		lines := strings.Split(out, "\n")

		if len(m.live.nodeRects) == 0 {
			t.Fatalf("[%s] View did not record any node rects", view)
		}
		for _, r := range m.live.nodeRects {
			if r.y < 0 || r.y >= len(lines) {
				t.Errorf("[%s] %s rect row %d is off-screen (%d lines)", view, r.name, r.y, len(lines))
				continue
			}
			row := []rune(lines[r.y])
			if r.x < 0 || r.x >= len(row) {
				t.Errorf("[%s] %s rect col %d is off the row (len %d)", view, r.name, r.x, len(row))
				continue
			}
			if row[r.x] != '╭' {
				t.Errorf("[%s] %s rect corner (%d,%d) lands on %q, not the card corner ╭", view, r.name, r.x, r.y, string(row[r.x]))
			}
		}
	}
}

func TestRecordNodeRectsSkipsOffice(t *testing.T) {
	m := twoNodeModel(t, vitals.StatusQuiet, vitals.StatusQuiet)
	m.recordNodeRects(3)
	if len(m.live.nodeRects) == 0 {
		t.Fatal("tree view should record rects")
	}
	m.view = viewOffice
	m.recordNodeRects(3)
	if len(m.live.nodeRects) != 0 {
		t.Errorf("office view should clear rects (cards not wired for click), got %d", len(m.live.nodeRects))
	}
}
