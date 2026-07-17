package lifecycle

import "fmt"

// Notice is one message that must be sent to a live agent to keep it
// coherent with a reparent that already happened. Reparenting is purely a
// metadata edit (spec §6.3) — the affected child has no way to know its
// manager changed unless CREW tells it.
//
// Delivery is not instant and this function does not pretend otherwise: per
// spec §13.1 (turn-boundary batching, confirmed empirically — a channel
// message cannot preempt a busy agent, only surface at its next turn
// boundary), a notice sent here may sit unseen for as long as the recipient
// stays busy. An agent can run for minutes on a stale org model between the
// reparent happening and the notice actually being read. That's a real
// substrate limitation to design around, not something this function can
// paper over by pretending the notice lands the moment it's produced.
type Notice struct {
	To      string
	Message string
}

// ReparentNotices produces one notice per move, addressed to the moved node.
func ReparentNotices(moves []Move) []Notice {
	notices := make([]Notice, 0, len(moves))
	for _, m := range moves {
		var msg string
		if m.NewParentID == "" {
			msg = "your manager was removed; you now report to no one (you're a root)"
		} else {
			msg = fmt.Sprintf("your manager was removed; you now report to %s", m.NewParentID)
		}
		notices = append(notices, Notice{To: m.NodeID, Message: msg})
	}
	return notices
}
