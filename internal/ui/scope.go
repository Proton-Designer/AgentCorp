package ui

import (
	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
)

// ScopedPeers wraps a raw peer source so it yields only the peers whose working
// directory belongs to the company rooted at companyRoot (nearest-wins, per
// company.Member). This is the single chokepoint that turns claude-peers' flat,
// machine-wide mesh into one company's view: everything downstream — reconcile,
// unmanaged detection, the HUD, layout — sees only in-company peers because
// they never receive the rest.
//
// companyRoot == "" means unscoped: the raw source is returned unchanged, so a
// directory that was never linked to a company still shows every session on the
// machine, exactly as before this feature existed.
//
// A raw-read failure propagates unchanged (unknown, never empty). A per-peer
// membership check that itself faults — a peer whose cwd can't be resolved —
// excludes that one peer rather than failing the whole read: one unreadable
// working directory must not blank out the company.
func ScopedPeers(companyRoot string, raw func() ([]broker.Peer, error)) func() ([]broker.Peer, error) {
	if companyRoot == "" {
		return raw
	}
	return func() ([]broker.Peer, error) {
		peers, err := raw()
		if err != nil {
			return nil, err
		}
		kept := make([]broker.Peer, 0, len(peers))
		for _, p := range peers {
			ok, err := company.Member(companyRoot, p.CWD)
			if err != nil {
				continue // can't confirm membership -> not shown, but don't fail the read
			}
			if ok {
				kept = append(kept, p)
			}
		}
		return kept, nil
	}
}
