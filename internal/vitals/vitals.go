// Package vitals turns raw peers+nodes+messages into what the hero screen's
// HUD and ticker display (spec §5, REQUIREMENTS UI-4/UI-5). Every function
// here is pure — same discipline as layout/ and broker.Reconcile, for the
// same reason: a subtle counting bug in a once-a-second HUD is nearly
// invisible, and only a deterministic table test catches it.
package vitals

import (
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Summary is the vitals HUD's derived state (spec §5, REQUIREMENTS UI-4).
//
// Active/Quiet replace the earlier working/idle/blocked concept (spec
// §5.2, decided after Working/Idle/Blocked were traced and found
// undeliverable — see vitals.go's git history for the reasoning). Active
// means "this node's peer sent a claude-peers message within `window` of
// now" — message recency, not inferred effort. It is honest precisely
// because it claims nothing more than that.
type Summary struct {
	Alive     int // bound nodes whose peer is currently live: Active + Quiet
	Active    int // Alive nodes that spoke within `window` of now
	Quiet     int // Alive nodes that did not
	Dead      int
	Unmanaged int
	Uptime    time.Duration
}

// Vitals computes the HUD summary from a point-in-time snapshot. now and
// window are explicit parameters, never internal state or a hardcoded
// constant — same purity discipline as Throughput, and window specifically
// must stay caller-supplied so the activity cutoff (spec §5.2 measured a
// starting point around 60s, but it's a tunable, not a fact) never gets
// silently baked into this package.
func Vitals(nodes []store.Node, peers []broker.Peer, msgs []broker.Message, now time.Time, window time.Duration) Summary {
	livePeerIDs := make(map[string]bool, len(peers))
	for _, p := range peers {
		livePeerIDs[p.ID] = true
	}
	lastSpoke := lastMessageByPeer(msgs)

	var s Summary
	var earliest time.Time
	for _, n := range nodes {
		if t, ok := parseTimestamp(n.CreatedAt); ok {
			if earliest.IsZero() || t.Before(earliest) {
				earliest = t
			}
		}

		switch {
		case n.State == "dead":
			s.Dead++
		case n.PeerID != "" && livePeerIDs[n.PeerID]:
			s.Alive++
			if isActive(n.PeerID, lastSpoke, now, window) {
				s.Active++
			} else {
				s.Quiet++
			}
		case n.PeerID != "":
			// Bound, but the peer is currently absent from the broker and
			// the DB hasn't caught up to 'dead' yet (sync/ reconciles on a
			// tick; this snapshot may be between ticks). A live HUD must
			// not call this Alive just because the DB hasn't updated.
			s.Dead++
			// n.PeerID == "" (pending/failed): counted in neither bucket. It's
			// not alive, not dead, and not unmanaged — it's still forming.
		}
	}

	// Reuse Reconcile's Unmanaged definition rather than recomputing a
	// second, subtly different notion of "unmanaged" here: Reconcile
	// already excludes peers claimed by a pending bind (see its own doc
	// comment) so a peer about to be auto-bound doesn't inflate this count
	// either.
	s.Unmanaged = len(broker.Reconcile(nodes, peers).Unmanaged)

	if !earliest.IsZero() {
		s.Uptime = now.Sub(earliest)
	}

	return s
}
