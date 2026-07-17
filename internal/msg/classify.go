package msg

import (
	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/store"
)

// Origin is a message's trust classification.
type Origin int

const (
	Known   Origin = iota // from_id is a peer currently bound to one of our nodes
	Unknown               // from_id is a real live peer, just not ours
	Forged                // from_id matches no currently live peer at all
)

func (o Origin) String() string {
	switch o {
	case Known:
		return "known"
	case Unknown:
		return "unknown"
	case Forged:
		return "forged"
	default:
		return "?"
	}
}

// Classify determines a message's origin trust level. This is REQUIREMENTS
// SE-2: CREW surfaces forged messages, it does not and cannot block them —
// the substrate has zero sender authentication (from_id is client-supplied
// and unverified; the bundled CLI literally hardcodes from_id:"cli", an
// identity that was never registered as a peer).
//
// Signature note: the brief specified Classify(msg, nodes) Origin, but
// distinguishing "unknown" (a real live peer, not ours) from "forged"
// (matches no live peer at all) is impossible with only our own node list —
// both look identical without knowing the CURRENT live peer set. Extended to
// take livePeers (e.g. from broker.ListPeers) rather than guess at a
// two-input version that couldn't actually produce the three-way
// distinction it's supposed to.
//
// livePeers must be current, not historical: a peer that was ours and has
// since gone is exactly as unverifiable as any other identity nobody is
// currently answering to.
func Classify(m broker.Message, nodes []store.Node, livePeers []broker.Peer) Origin {
	live := make(map[string]bool, len(livePeers))
	for _, p := range livePeers {
		live[p.ID] = true
	}
	if !live[m.FromID] {
		return Forged
	}
	for _, n := range nodes {
		if n.PeerID == m.FromID {
			return Known
		}
	}
	return Unknown
}
