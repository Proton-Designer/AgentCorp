package broker

import "github.com/aymanmohammed/crew/internal/store"

// Binding is a proposed pending -> alive bind: a pending node whose recorded
// bind_tty matches a live peer's tty. The caller performs the actual write
// via (*store.Store).BindPeer, which independently re-checks that the node is
// still pending — Reconcile only proposes.
type Binding struct {
	NodeID string
	PeerID string
}

// Reconciliation is the pure diff between our sidecar store and the broker's
// live peer list (spec §9's "Reconciliation on launch"). The caller performs
// every write; this function performs none.
type Reconciliation struct {
	Alive       []store.Node // bound nodes whose peer is still live — no action
	Dead        []store.Node // bound nodes whose peer is gone — caller tombstones
	Unmanaged   []Peer       // live peers with no node pointing at them — not ours until adopted
	PendingBind []Binding    // pending nodes whose bind_tty matches a live peer — caller binds
}

// Reconcile is a pure function: nodes and peers in, a diff out. No I/O, no
// store calls, no broker calls — same discipline as internal/layout/, for the
// same reason: this is the second-highest-risk logic in the project (spec
// §9's reconciliation state machine) and purity is what makes it exhaustively
// table-testable.
//
// Unmanaged is never persisted here or by any caller obligation this
// function implies — it's spec §9's computed diff, not a row. A peer only
// becomes a node when an operator explicitly adopts it.
func Reconcile(nodes []store.Node, peers []Peer) Reconciliation {
	livePeers := make(map[string]Peer, len(peers))
	for _, p := range peers {
		livePeers[p.ID] = p
	}

	// A peer_id counts as "managed" the moment any node was ever bound to it,
	// regardless of that node's current state — a tombstoned node's old
	// peer_id must not make a live peer with the same id look unmanaged.
	// (In practice this never collides: peer ids are freshly random per
	// registration, so a dead session's old id is never reissued.)
	boundPeerIDs := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if n.PeerID != "" {
			boundPeerIDs[n.PeerID] = true
		}
	}

	var rec Reconciliation
	for _, n := range nodes {
		switch {
		case n.PeerID != "":
			if _, alive := livePeers[n.PeerID]; alive {
				rec.Alive = append(rec.Alive, n)
			} else if n.State != "dead" {
				// Bound, but the peer is gone and we haven't recorded that yet.
				rec.Dead = append(rec.Dead, n)
			}
			// n.State == "dead" already: no action, already tombstoned.

		case n.State == "pending" && n.BindTTY != "":
			target := NormalizeTTY(n.BindTTY)
			for _, p := range peers {
				if NormalizeTTY(p.TTY) == target {
					rec.PendingBind = append(rec.PendingBind, Binding{NodeID: n.NodeID, PeerID: p.ID})
					break
				}
			}
		}
	}

	// A peer about to be claimed via PendingBind is spoken for, even though
	// no node.PeerID points at it yet — it must not also render as
	// Unmanaged, or the UI would offer the same peer as both "adopt me" and
	// "about to be auto-bound" at once.
	claimedByPendingBind := make(map[string]bool, len(rec.PendingBind))
	for _, b := range rec.PendingBind {
		claimedByPendingBind[b.PeerID] = true
	}

	for _, p := range peers {
		if !boundPeerIDs[p.ID] && !claimedByPendingBind[p.ID] {
			rec.Unmanaged = append(rec.Unmanaged, p)
		}
	}

	return rec
}
