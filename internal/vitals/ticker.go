package vitals

import (
	"fmt"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

// tickerMaxRunes bounds the excerpt shown in the ticker line. Not spec'd to
// an exact number; chosen to keep a single-line ticker readable on an
// 80-column terminal alongside the "from → to  " prefix and timestamp.
const tickerMaxRunes = 60

// Ticker formats the single most recent message for the activity ticker
// (spec §5, REQUIREMENTS UI-5): `from → to  "text…"`. Returns "" for an
// empty msgs slice.
//
// msgs must already be ordered oldest-first — ListMessages' contract — so
// Ticker takes the last element rather than re-sorting; it has no way to
// verify the ordering itself without becoming impure.
func Ticker(msgs []broker.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	m := msgs[len(msgs)-1]
	return fmt.Sprintf("%s → %s  %q", m.FromID, m.ToID, truncate(m.Text, tickerMaxRunes))
}

// truncate cuts s to at most max runes, appending "…" if it was cut.
// Rune-based, not byte-based, so multi-byte UTF-8 is never split mid-character.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
