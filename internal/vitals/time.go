package vitals

import "time"

// parseTimestamp parses a claude-peers/CREW timestamp. Verified against live
// data: the broker stores RFC3339 with millisecond fractional seconds (e.g.
// "2026-07-17T02:14:40.469Z", from `sqlite3 ~/.claude-peers.db`), while this
// project's own store fixtures use bare RFC3339 with no fractional part
// (e.g. "2026-07-16T00:00:00Z"). time.RFC3339Nano's fractional-second
// pattern is optional-digit, so a single layout parses both.
func parseTimestamp(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
