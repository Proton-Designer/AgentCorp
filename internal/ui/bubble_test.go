package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

func bubbleModel(t *testing.T, msgs []broker.Message, sel int) Model {
	t.Helper()
	nodes := []store.Node{
		{NodeID: "1", Name: "CEO", PeerID: "pC", CreatedAt: "2026-01-01T00:00:00Z"},
		{NodeID: "2", Name: "backend", ParentID: "1", PeerID: "pB", CreatedAt: "2026-01-01T00:00:01Z"},
	}
	m := New(nodes)
	m.width = 100
	m.motion = motionCalm
	m.live = &liveState{
		msgs: msgs,
		peers: []broker.Peer{
			{ID: "pC", Summary: "steering the roadmap"},
			{ID: "pB", Summary: "refactoring the ledger"},
		},
		statuses:   map[string]vitals.Status{"CEO": vitals.StatusActive, "backend": vitals.StatusQuiet},
		nameToPeer: map[string]string{"CEO": "pC", "backend": "pB"},
	}
	m.cursor = sel
	return m
}

func lineWidths(s string) []int {
	var w []int
	for _, ln := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		w = append(w, len([]rune(ln)))
	}
	return w
}

func TestBubbleLinesAlign(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false // width == rune count only with no ANSI in the string

	now := time.Now()
	m := bubbleModel(t, []broker.Message{
		{FromID: "pC", ToID: "pB", Text: "on it", SentAt: now.Add(-3 * time.Second).UTC().Format(time.RFC3339Nano)},
	}, 0)

	out := m.renderSpeechBubble()
	widths := lineWidths(out)
	if len(widths) != 3 {
		t.Fatalf("bubble should be 3 lines, got %d:\n%s", len(widths), out)
	}
	if widths[0] != widths[1] || widths[1] != widths[2] {
		t.Errorf("bubble lines must be equal width (box aligns): %v\n%s", widths, out)
	}
	if widths[0] > m.width {
		t.Errorf("bubble line %d exceeds width %d", widths[0], m.width)
	}
}

func TestBubbleShowsHonestAge(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	now := time.Now()

	fresh := bubbleModel(t, []broker.Message{
		{FromID: "pC", ToID: "pB", Text: "shipped", SentAt: now.Add(-3 * time.Second).UTC().Format(time.RFC3339Nano)},
	}, 0)
	if out := fresh.renderSpeechBubble(); !strings.Contains(out, "said 3s ago") || !strings.Contains(out, "\"shipped\"") {
		t.Errorf("fresh bubble must quote the message and its age:\n%s", out)
	}

	old := bubbleModel(t, []broker.Message{
		{FromID: "pC", ToID: "pB", Text: "old news", SentAt: now.Add(-3 * time.Minute).UTC().Format(time.RFC3339Nano)},
	}, 0)
	if out := old.renderSpeechBubble(); !strings.Contains(out, "quiet for 3m") {
		t.Errorf("a stale last-message must read as history, not recent speech:\n%s", out)
	}
}

func TestBubbleNoMessageShowsSummaryMarked(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	// backend (index 1) has no sent messages in this stream.
	m := bubbleModel(t, []broker.Message{
		{FromID: "pC", ToID: "pB", Text: "hi", SentAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}, 1)
	out := m.renderSpeechBubble()
	if !strings.Contains(out, "no messages yet") {
		t.Errorf("an agent that hasn't spoken must say so, not fake a quote:\n%s", out)
	}
	if !strings.Contains(out, "~ refactoring the ledger") {
		t.Errorf("its self-summary should show, marked with ~:\n%s", out)
	}
}

func TestBubbleNoANSIWhenColorDisabled(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	m := bubbleModel(t, []broker.Message{
		{FromID: "pC", ToID: "pB", Text: "x", SentAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}, 0)
	if strings.Contains(m.renderSpeechBubble(), "\x1b") {
		t.Errorf("no ESC bytes allowed when colour is disabled")
	}
}
