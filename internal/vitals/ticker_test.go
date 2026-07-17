package vitals

import (
	"strings"
	"testing"

	"github.com/aymanmohammed/crew/internal/broker"
)

// All fixture text below is synthetic, authored for this test — never real
// message content read from a live broker.
func TestTickerEmptyReturnsEmptyString(t *testing.T) {
	if got := Ticker(nil); got != "" {
		t.Fatalf("Ticker(nil) = %q, want empty string", got)
	}
}

func TestTickerUsesLastMessage(t *testing.T) {
	msgs := []broker.Message{
		{FromID: "a", ToID: "b", Text: "first"},
		{FromID: "c", ToID: "d", Text: "second"},
	}
	got := Ticker(msgs)
	if !strings.Contains(got, "c") || !strings.Contains(got, "d") || !strings.Contains(got, "second") {
		t.Fatalf("Ticker(msgs) = %q, want it to reflect the last message (c -> d, second), not the first", got)
	}
	if strings.Contains(got, "first") {
		t.Fatalf("Ticker(msgs) = %q, must not include the earlier message's text", got)
	}
}

func TestTickerFormatsFromArrowTo(t *testing.T) {
	got := Ticker([]broker.Message{{FromID: "lead-be", ToID: "backend-dev", Text: "take /bookings"}})
	want := `lead-be → backend-dev  "take /bookings"`
	if got != want {
		t.Fatalf("Ticker = %q, want %q", got, want)
	}
}

func TestTickerTruncatesLongText(t *testing.T) {
	long := strings.Repeat("x", tickerMaxRunes+20)
	got := Ticker([]broker.Message{{FromID: "a", ToID: "b", Text: long}})
	if strings.Contains(got, "…") == false {
		t.Fatalf("Ticker with over-long text = %q, want an ellipsis marking truncation", got)
	}
	if strings.Contains(got, long) {
		t.Fatal("Ticker did not truncate the long text")
	}
}

func TestTruncateIsRuneSafe(t *testing.T) {
	// Multi-byte runes: truncating by byte count would split a character
	// and produce invalid UTF-8 or a garbled glyph.
	s := strings.Repeat("é", 10) // each 'é' is 2 bytes in UTF-8
	got := truncate(s, 5)
	wantRunes := 5 + 1 // 5 kept + the ellipsis rune
	if got != strings.Repeat("é", 5)+"…" {
		t.Fatalf("truncate = %q, want 5 é's plus an ellipsis", got)
	}
	if len([]rune(got)) != wantRunes {
		t.Fatalf("truncate result has %d runes, want %d", len([]rune(got)), wantRunes)
	}
}

func TestTruncateNoOpWhenShortEnough(t *testing.T) {
	if got := truncate("short", 60); got != "short" {
		t.Fatalf("truncate(\"short\", 60) = %q, want unchanged", got)
	}
}
