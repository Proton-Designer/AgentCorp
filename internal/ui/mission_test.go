package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

func TestPanelBoxDimensions(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	lines := panelBox("TITLE", []string{"one", "two"}, 20, 5, styConnector)
	if len(lines) != 5 {
		t.Fatalf("panel of height 5 must be 5 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if vw := visibleWidth(ln); vw != 20 {
			t.Errorf("line %d width %d, want 20: %q", i, vw, ln)
		}
	}
	if !strings.Contains(lines[0], "TITLE") {
		t.Errorf("top border should carry the title: %q", lines[0])
	}
}

func TestPadVisible(t *testing.T) {
	if got := padVisible("hi", 5); got != "hi   " {
		t.Errorf("pad short to width, got %q", got)
	}
	if got := padVisible("toolongtext", 4); got != "tool" {
		t.Errorf("clip plain text to width, got %q", got)
	}
}

func missionModel() Model {
	nodes := []store.Node{
		{NodeID: "1", Name: "CEO", PeerID: "pC", CreatedAt: "2026-01-01T00:00:00Z"},
		{NodeID: "2", Name: "backend", ParentID: "1", PeerID: "pB", CreatedAt: "2026-01-01T00:00:01Z"},
	}
	m := New(nodes)
	m.width, m.height = 100, 30
	m.live = &liveState{
		summary:    vitals.Summary{Alive: 2, Active: 1, Quiet: 1},
		spark:      "▁▂▅█",
		msgs:       []broker.Message{{FromID: "pC", ToID: "pB", Text: "on it"}},
		peers:      []broker.Peer{{ID: "pC"}, {ID: "pB"}},
		statuses:   map[string]vitals.Status{"CEO": vitals.StatusActive, "backend": vitals.StatusQuiet},
		peerToName: map[string]string{"pC": "CEO", "pB": "backend"},
	}
	return m
}

func TestMissionFitsWidth(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	m := missionModel()
	const w = 100
	out := m.renderMission(w, 22)
	for _, ln := range strings.Split(out, "\n") {
		if vw := visibleWidth(ln); vw > w {
			t.Errorf("mission line exceeds width %d: %d cells: %q", w, vw, ln)
		}
	}
	if !strings.Contains(out, "ORG") || !strings.Contains(out, "VITALS") || !strings.Contains(out, "ALERTS") {
		t.Errorf("mission view should have ORG, VITALS, ALERTS panels")
	}
}

func TestMissionAlertsAllClear(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	m := missionModel()
	alerts := m.missionAlerts(30)
	if len(alerts) != 1 || !strings.Contains(alerts[0], "all clear") {
		t.Errorf("no dead/stale/unmanaged should read 'all clear', got %v", alerts)
	}
	// Add a dead agent and it must surface.
	m.live.statuses["backend"] = vitals.StatusDead
	alerts = m.missionAlerts(30)
	joined := strings.Join(alerts, "|")
	if !strings.Contains(joined, "dead") || !strings.Contains(joined, "backend") {
		t.Errorf("a dead agent must be surfaced as an alert, got %v", alerts)
	}
}
