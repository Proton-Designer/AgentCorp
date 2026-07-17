package sync

import (
	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Actions is what one tick decided needs to happen to the store. Pure —
// derived entirely from a PaneDiff and a broker.Reconciliation, no I/O.
type Actions struct {
	Tombstone []string         // node IDs whose peer or tmux pane is gone
	Bind      []broker.Binding // pending nodes to bind to a newly-seen peer
}

// Decide is the pure reconcile-decision step, combining two independent
// death signals per spec §9's hard contract:
//
//	node.SpawnRef == ""               -> pane-diff says nothing about it; broker signal only
//	node.SpawnRef != "" && pane gone  -> dead (tmux signal)
//	node.PeerID   != "" && peer gone  -> dead (broker signal)
//
// Either signal is sufficient; neither is required. The tmux signal is
// typically FASTER (a pane's disappearance is near-instant; the broker's own
// PID-reap runs on its own cadence), which is why it's a second signal and
// not a redundant one — see TestDecideTombstonesViaPaneDeathSignalAlone....
//
// nodes is passed separately from recon because Reconciliation doesn't
// expose spawn_ref (only Alive/Dead/Unmanaged/PendingBind).
//
// The empty-SpawnRef guard is not defensive boilerplate: on a real dev box,
// most peers have no tmux pane at all (measured: 7 broker peers, 1 tmux
// pane), so a node with no pane is the common case, not an edge case, and
// must never be tombstoned by a signal it was never eligible for.
func Decide(diff PaneDiff, recon broker.Reconciliation, nodes []store.Node) Actions {
	var a Actions
	tombstoned := map[string]bool{}

	for _, n := range recon.Dead {
		a.Tombstone = append(a.Tombstone, n.NodeID)
		tombstoned[n.NodeID] = true
	}

	diedPanes := make(map[string]bool, len(diff.Died))
	for _, id := range diff.Died {
		diedPanes[id] = true
	}
	for _, n := range nodes {
		if n.SpawnRef == "" || n.State == "dead" || tombstoned[n.NodeID] {
			continue
		}
		if diedPanes[n.SpawnRef] {
			a.Tombstone = append(a.Tombstone, n.NodeID)
			tombstoned[n.NodeID] = true
		}
	}

	a.Bind = append(a.Bind, recon.PendingBind...)
	return a
}
