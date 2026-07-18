package ui

import (
	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
)

// InCompany returns the subset of peers whose working directory belongs to the
// company rooted at companyRoot (nearest-wins, per company.Member).
//
// This is a DISPLAY filter, and only a display filter. It is used to scope the
// "unmanaged" adoption list to one company so a machine running many unrelated
// sessions doesn't offer them all for adoption. It is deliberately NOT used to
// decide whether an already-bound node is alive: a node we own lives or dies by
// the real broker, never by whether a per-tick filesystem resolution happened
// to include its peer this instant. Coupling liveness to this filter is exactly
// what let a single transient resolution blip tombstone a live agent forever.
//
// companyRoot == "" (unscoped) returns peers unchanged. A peer whose membership
// can't be resolved is excluded from the scoped view — it is not shown, but it
// is never treated as dead.
func InCompany(companyRoot string, peers []broker.Peer) []broker.Peer {
	if companyRoot == "" {
		return peers
	}
	kept := make([]broker.Peer, 0, len(peers))
	for _, p := range peers {
		ok, err := company.Member(companyRoot, p.CWD)
		if err == nil && ok {
			kept = append(kept, p)
		}
	}
	return kept
}

// scopeMessages keeps only the messages that belong to this company's activity:
// sent by or to an in-company peer, or sent by the console itself ("agentcorp").
// Without this the HUD's sparkline and ticker count every unrelated session's
// traffic on the machine — on a busy laptop that drowns out the company's own
// signal entirely. companyRoot == "" (unscoped) returns msgs unchanged.
func scopeMessages(companyRoot string, peers []broker.Peer, msgs []broker.Message) []broker.Message {
	if companyRoot == "" {
		return msgs
	}
	inCompany := make(map[string]bool)
	for _, p := range InCompany(companyRoot, peers) {
		inCompany[p.ID] = true
	}
	kept := make([]broker.Message, 0, len(msgs))
	for _, m := range msgs {
		if inCompany[m.FromID] || inCompany[m.ToID] || m.FromID == msgSenderConsole {
			kept = append(kept, m)
		}
	}
	return kept
}

// msgSenderConsole is the from_id the console stamps on operator messages — its
// own identity, surfaced honestly rather than spoofing a peer.
const msgSenderConsole = "agentcorp"
