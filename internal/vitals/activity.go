package vitals

import (
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

// lastMessageByPeer returns, for each from_id appearing in msgs, the
// timestamp of that peer's most recent sent message. Only FromID is
// indexed: a message TO a peer says nothing about that peer's own activity,
// only the sender's. Shared by Vitals and NodeStatus so both derive
// "active" from the identical rule rather than two implementations that can
// drift apart.
func lastMessageByPeer(msgs []broker.Message) map[string]time.Time {
	last := make(map[string]time.Time, len(msgs))
	for _, m := range msgs {
		t, ok := parseTimestamp(m.SentAt)
		if !ok {
			continue
		}
		if prev, exists := last[m.FromID]; !exists || t.After(prev) {
			last[m.FromID] = t
		}
	}
	return last
}

// isActive reports whether peerID has a recorded last-spoke time within
// window of now. A message timestamped after now (clock skew, bad data) is
// treated as not-active rather than active — an anomaly should never read
// as extra-fresh activity.
func isActive(peerID string, lastSpoke map[string]time.Time, now time.Time, window time.Duration) bool {
	t, ok := lastSpoke[peerID]
	if !ok || now.Before(t) {
		return false
	}
	return now.Sub(t) <= window
}
