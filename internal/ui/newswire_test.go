package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

func TestNewswireBandResolvesNames(t *testing.T) {
	nameOf := func(id string) string {
		switch id {
		case "pC":
			return "CEO"
		case "pB":
			return "backend"
		}
		return id // unknown → raw id, honestly
	}
	msgs := []broker.Message{
		{FromID: "pC", ToID: "pB", Text: "on it"},
		{FromID: "pB", ToID: "pX", Text: "done"}, // pX unknown
	}
	band := newswireBand(msgs, nameOf, 10)
	if !strings.Contains(band, `CEO -> backend: "on it"`) {
		t.Errorf("band should show resolved names: %q", band)
	}
	if !strings.Contains(band, `backend -> pX: "done"`) {
		t.Errorf("unknown peer should fall back to its id: %q", band)
	}
	if strings.Contains(band, "\x1b") {
		t.Errorf("band is plain text; colour is applied at render, not here")
	}
	if newswireBand(nil, nameOf, 10) != "" {
		t.Errorf("no messages → empty band")
	}
}

func TestMarqueeWindowWrapsSeamlessly(t *testing.T) {
	band := "ABCDEFGH"
	// Shorter-or-equal than width: returned as-is.
	if got := marqueeWindow(band, 20, 0); got != band {
		t.Errorf("short band should be returned whole, got %q", got)
	}
	// Exactly width runes at any offset.
	for off := 0; off < 40; off++ {
		w := marqueeWindow(band, 4, off)
		if n := len([]rune(w)); n != 4 {
			t.Fatalf("window at offset %d has %d runes, want 4 (%q)", off, n, w)
		}
	}
	// Advancing the offset shifts the window by one (scrolls).
	a := marqueeWindow(band, 4, 0)
	b := marqueeWindow(band, 4, 1)
	if a == b {
		t.Errorf("offset change should scroll the window: %q == %q", a, b)
	}
}

func TestPulseMonitorSpikesAtActivity(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	bucket := 100 * time.Millisecond
	width := 20
	// One message ~now (rightmost columns), none older → last column tallest.
	msgs := []broker.Message{
		{SentAt: now.Add(-50 * time.Millisecond).UTC().Format(time.RFC3339Nano)},
	}
	strip := pulseMonitor(msgs, now, width, bucket)
	r := []rune(strip)
	if len(r) != width {
		t.Fatalf("pulse strip must be exactly width runes, got %d", len(r))
	}
	if r[width-1] == pulseChars[0] {
		t.Errorf("a message at ~now should raise the rightmost column above baseline")
	}
	// Only unambiguous block glyphs are used.
	for _, c := range r {
		ok := false
		for _, p := range pulseChars {
			if c == p {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("pulse used a non-block glyph %q (width-unsafe)", c)
		}
	}
	// No messages → flatline of the lowest block, exactly width wide.
	flat := pulseMonitor(nil, now, width, bucket)
	if flat != strings.Repeat(string(pulseChars[0]), width) {
		t.Errorf("empty history must flatline, got %q", flat)
	}
}
