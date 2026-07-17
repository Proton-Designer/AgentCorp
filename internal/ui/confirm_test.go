package ui

import (
	"strings"
	"testing"

	"github.com/aymanmohammed/crew/internal/store"
	"github.com/aymanmohammed/crew/internal/vitals"
)

func nd(id, name, role string) store.Node {
	return store.Node{NodeID: id, Name: name, Role: role,
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: id}
}

// §6.3: the disband dialog must enumerate EVERY process that will die.
// "Disband lead-be's team" tells the operator nothing about what they're
// destroying — the list is the point.
func TestDisbandDialogEnumeratesEveryCasualty(t *testing.T) {
	c := &confirmState{
		kind:   confirmDisband,
		victim: nd("2", "lead-be", "backend"),
		casualties: []store.Node{
			nd("4", "backend-dev", "dev"),
			nd("5", "db-dev", "dev"),
			nd("2", "lead-be", "backend"),
		},
		statuses: map[string]vitals.Status{},
	}
	out := renderConfirm(c, 80)
	for _, name := range []string{"backend-dev", "db-dev", "lead-be"} {
		if !strings.Contains(out, name) {
			t.Fatalf("disband dialog does not name %q — the operator cannot see what dies:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "3 session") {
		t.Fatalf("dialog does not state the count:\n%s", out)
	}
}

// Active agents must be flagged. Losing a working agent's progress is the
// specific harm the confirm exists to prevent.
func TestDisbandDialogFlagsActiveAgents(t *testing.T) {
	c := &confirmState{
		kind:   confirmDisband,
		victim: nd("2", "lead-be", "backend"),
		casualties: []store.Node{
			nd("4", "backend-dev", "dev"),
			nd("5", "db-dev", "dev"),
		},
		statuses: map[string]vitals.Status{
			"4": vitals.StatusActive,
			"5": vitals.StatusQuiet,
		},
	}
	out := renderConfirm(c, 80)
	if !strings.Contains(out, "⚠ active") {
		t.Fatalf("active casualty not flagged:\n%s", out)
	}
	if !strings.Contains(out, "1 agent(s) are active and will lose their work") {
		t.Fatalf("dialog does not warn about active work:\n%s", out)
	}
}

// Fire must state plainly that reports SURVIVE. Reparent-not-cascade is
// worthless if the operator can't tell which one they're about to do.
func TestFireDialogSaysReportsSurvive(t *testing.T) {
	c := &confirmState{
		kind:      confirmFire,
		victim:    nd("2", "lead-be", "backend"),
		moves:     2,
		newParent: "ceo",
	}
	out := renderConfirm(c, 80)
	if !strings.Contains(out, "survive") {
		t.Fatalf("fire dialog does not say reports survive:\n%s", out)
	}
	if !strings.Contains(out, "ceo") {
		t.Fatalf("fire dialog does not say where reports land:\n%s", out)
	}
	if !strings.Contains(out, "Terminates 1 session") {
		t.Fatalf("fire dialog does not scope the kill to one session:\n%s", out)
	}
}

// The two dialogs must be visibly different actions. If fire looked like
// disband, the operator would learn to dismiss both the same way.
func TestFireAndDisbandAreDistinctDialogs(t *testing.T) {
	victim := nd("2", "lead-be", "backend")
	fire := renderConfirm(&confirmState{kind: confirmFire, victim: victim, moves: 2, newParent: "ceo"}, 80)
	disband := renderConfirm(&confirmState{
		kind: confirmDisband, victim: victim,
		casualties: []store.Node{nd("4", "backend-dev", "dev")},
		statuses:   map[string]vitals.Status{},
	}, 80)

	if fire == disband {
		t.Fatal("fire and disband render identically")
	}
	if strings.Contains(fire, "Disband") {
		t.Fatal("fire dialog mentions disband — the actions must not blur")
	}
	// Disband requires a distinct, harder keystroke.
	if !strings.Contains(disband, "shift-D") {
		t.Fatalf("disband is not gated behind a distinct key:\n%s", disband)
	}
	if strings.Contains(disband, "[ f ] fire") {
		t.Fatal("disband dialog offers the fire key")
	}
}

// A fire with no children must not claim reports survive.
func TestFireDialogWithNoReports(t *testing.T) {
	out := renderConfirm(&confirmState{
		kind: confirmFire, victim: nd("4", "backend-dev", "dev"), moves: 0,
	}, 80)
	if strings.Contains(out, "survive") {
		t.Fatalf("fire dialog claims reports survive when there are none:\n%s", out)
	}
	if !strings.Contains(out, "No reports") {
		t.Fatalf("dialog does not say there are no reports:\n%s", out)
	}
}
