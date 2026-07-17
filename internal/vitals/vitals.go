// Package vitals turns raw peers+nodes+messages into what the hero screen's
// HUD and ticker display (spec §5, REQUIREMENTS UI-4/UI-5). Every function
// here is pure — same discipline as layout/ and broker.Reconcile, for the
// same reason: a subtle counting bug in a once-a-second HUD is nearly
// invisible, and only a deterministic table test catches it.
package vitals

import (
	"time"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/store"
)

// Summary is the vitals HUD's derived state (spec §5, REQUIREMENTS UI-4).
//
// Working, Idle, and Blocked are NOT computed in this version and are
// always zero — see the doc comment on Vitals for why. Treat these three
// fields as "unknown," not "confirmed zero," until that gap is resolved.
// Alive, Dead, Unmanaged, and Uptime are real.
type Summary struct {
	Alive     int
	Working   int
	Idle      int
	Blocked   int
	Dead      int
	Unmanaged int
	Uptime    time.Duration
}

// Vitals computes the HUD summary from a point-in-time snapshot of nodes and
// peers. now is explicit rather than time.Now() for the same purity reason
// as Throughput (see its doc comment): a deterministic table test is the
// only thing that reliably catches a bucketing or counting bug here.
//
// Working/Idle/Blocked are deliberately left at zero — not computed via a
// keyword heuristic. claude-peers exposes exactly two pieces of live state
// per peer: a free-text `summary` (either LLM-guessed from git context at
// startup, or set ad hoc by the agent via set_summary — no schema, no
// forced update cadence) and `last_seen`, which ticks on a fixed 15s
// heartbeat regardless of whether the agent is mid-turn or idle at a
// prompt (reference doc §5). Neither is a reliable signal for "is this
// agent currently working." Matching keywords like "waiting"/"idle" in
// summary text would produce a number that looks precise and isn't —
// exactly the failure mode called out when this task was assigned. A real
// signal would need either a status convention CREW imposes on nodes it
// spawns (via --append-system-prompt, which only ever covers CREW-spawned
// nodes, never adopted ones) or an explicit decision that this can't be
// shown in v1. That decision isn't mine to make unilaterally, so it's
// surfaced, not guessed.
func Vitals(nodes []store.Node, peers []broker.Peer, now time.Time) Summary {
	livePeerIDs := make(map[string]bool, len(peers))
	for _, p := range peers {
		livePeerIDs[p.ID] = true
	}

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
