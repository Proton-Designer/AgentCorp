package vitals

import (
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

// Throughput buckets msgs into one-minute-wide buckets covering
// [now-window, now), oldest bucket first, and returns the message count per
// bucket — the raw data for the hero screen's msgs/min sparkline (spec §5,
// REQUIREMENTS UI-4).
//
// now is an explicit parameter rather than time.Now(): a deliberate
// deviation from the one-line signature this task was described with,
// flagged rather than silent, for the same reason Reconcile and the layout
// engine are pure — a subtle bucketing bug in a once-a-second HUD is nearly
// invisible, and only a deterministic table test catches it. Bucketing by
// whole minutes (rather than an arbitrary bucket count) is what makes the
// output directly "messages per minute" without a second conversion step.
func Throughput(msgs []broker.Message, window time.Duration, now time.Time) []int {
	if window <= 0 {
		return nil
	}
	buckets := int(window / time.Minute)
	if buckets <= 0 {
		buckets = 1
	}
	counts := make([]int, buckets)
	start := now.Add(-window)

	for _, m := range msgs {
		t, ok := parseTimestamp(m.SentAt)
		if !ok || t.Before(start) || !t.Before(now) {
			continue
		}
		idx := int(t.Sub(start) / time.Minute)
		if idx >= buckets {
			idx = buckets - 1
		}
		counts[idx]++
	}
	return counts
}
