package sync

import (
	"fmt"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// TickMsg is one poll cycle's result. It has no dependency on Bubble Tea —
// tea.Msg is `interface{}`, so any concrete type satisfies it, and this
// package has no reason to import a UI framework to produce data for one.
//
// Err is set whenever ANY part of the tick failed (tmux unreachable, broker
// unreachable, store read failure). When Err != nil, Reconciliation/PaneDiff
// reflect only what was gathered before the failure — the UI must treat a
// non-nil Err as "unknown," never infer death or absence from it.
type TickMsg struct {
	PaneDiff       PaneDiff
	Reconciliation broker.Reconciliation
	Applied        Actions
	Err            error
}

// Tick runs one poll-diff-reconcile-apply cycle.
//
// listPanes and listPeers are injected rather than called directly so this
// can be tested without a live tmux server or a real claude-peers broker —
// the orchestration logic is what's under test here, not those two external
// systems (which have their own dedicated tests: the real-tmux integration
// test in tmux_test.go, and broker's own suite).
//
// The hard invariant, stated plainly because it's the expensive one to get
// wrong: any failed poll must change NOTHING in the store. A transient tmux
// or broker hiccup must never cascade into tombstoning live nodes — that's
// the single most expensive failure mode this package can produce, because
// tombstone is the one state this design treats as terminal. Both I/O calls
// are checked before any store write is attempted.
func Tick(
	listPanes func() (map[string]Pane, error),
	listPeers func() ([]broker.Peer, error),
	st *store.Store,
	lastPanes map[string]Pane,
) (TickMsg, map[string]Pane) {
	curPanes, err := listPanes()
	if err != nil {
		// Poll failed -- unknown, not empty. Don't advance lastPanes (we have
		// no new snapshot to advance to) and don't touch the store.
		return TickMsg{Err: fmt.Errorf("tick: %w", err)}, lastPanes
	}
	diff := DiffPanes(lastPanes, curPanes)

	peers, err := listPeers()
	if err != nil {
		// tmux side succeeded, so we DO advance lastPanes (that signal was
		// real and shouldn't be thrown away just because the broker call
		// failed independently) -- but we take no store action at all.
		return TickMsg{PaneDiff: diff, Err: fmt.Errorf("tick: %w", err)}, curPanes
	}

	nodes, err := st.ListNodes()
	if err != nil {
		return TickMsg{PaneDiff: diff, Err: fmt.Errorf("tick: %w", err)}, curPanes
	}

	recon := broker.Reconcile(nodes, peers)
	actions := Decide(diff, recon, nodes)

	if err := apply(st, actions); err != nil {
		return TickMsg{PaneDiff: diff, Reconciliation: recon, Err: fmt.Errorf("tick: apply: %w", err)}, curPanes
	}

	return TickMsg{PaneDiff: diff, Reconciliation: recon, Applied: actions}, curPanes
}

// apply is the I/O edge that turns Actions into store writes.
func apply(st *store.Store, a Actions) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, nodeID := range a.Tombstone {
		if err := st.Tombstone(nodeID, now); err != nil {
			return fmt.Errorf("tombstone %s: %w", nodeID, err)
		}
	}
	for _, b := range a.Bind {
		if err := st.BindPeer(b.NodeID, b.PeerID); err != nil {
			return fmt.Errorf("bind %s<-%s: %w", b.NodeID, b.PeerID, err)
		}
	}
	return nil
}
