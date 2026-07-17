package vitals

import (
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Status is a single node's derived classification, driving the hero
// screen's per-card glyph (spec §5, UI-3).
type Status string

const (
	StatusActive Status = "active" // ●  spoke within `window` of now
	StatusQuiet  Status = "quiet"  // ○  alive, hasn't spoken within window
	StatusDead   Status = "dead"   // dim  tombstoned, or bound peer vanished

	// StatusUnreachable corresponds to spec §5.1's `⚠` glyph ("possibly
	// unreachable"). It is declared here so the type is complete, but
	// NodeStatus never returns it: §5.1's heuristic is `ps -o ppid=` on the
	// peer's registered pid, then `ps -o command=` on its parent, to read
	// argv for the channel flag — inherently impure (shells out to the OS
	// process table), so it cannot live inside a pure (node, peers, msgs,
	// now, window) -> Status function. Computing it is a separate, impure
	// caller's job; that caller can layer StatusUnreachable on top of (or
	// instead of) whatever NodeStatus returns for an adopted node.
	StatusUnreachable Status = "unreachable"

	// StatusPending covers a node with no bound peer yet: a hire still in
	// flight (state=pending) or one that never bound (state=failed).
	// Not one of spec §5's four glyphs — the hire flow renders these
	// separately (§6.1) — but NodeStatus must be total over every
	// store.Node it can be handed, and none of Active/Quiet/Dead/
	// Unreachable honestly describes a node that was never bound. This is
	// an addition beyond the four statuses as first described; flagged; see
	// the task report for the full reasoning.
	StatusPending Status = "pending"
)

// NodeStatus classifies a single node against a live peer/message
// snapshot. Pure — same now/window parameters as Vitals, for the same
// reason: only a deterministic test catches a subtle classification bug in
// a per-card glyph.
//
// Rebuilds lastMessageByPeer(msgs) on every call, so calling it once per
// node in a render loop is O(nodes × messages). Fine at v1 scale (LY-6's
// 40-node/16ms budget is layout's, not this); if it ever shows up in a
// profile, hoist lastMessageByPeer(msgs) out and thread it through instead.
func NodeStatus(node store.Node, peers []broker.Peer, msgs []broker.Message, now time.Time, window time.Duration) Status {
	if node.State == "dead" {
		return StatusDead
	}
	if node.PeerID == "" {
		return StatusPending
	}

	live := false
	for _, p := range peers {
		if p.ID == node.PeerID {
			live = true
			break
		}
	}
	if !live {
		return StatusDead
	}

	if isActive(node.PeerID, lastMessageByPeer(msgs), now, window) {
		return StatusActive
	}
	return StatusQuiet
}
