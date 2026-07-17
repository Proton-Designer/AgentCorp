package sync

import (
	"errors"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func newTickTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/tick_test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestTickTombstonesDeadAndBindsPending(t *testing.T) {
	st := newTickTestStore(t)

	// A node bound to a peer that will be reported gone this tick.
	if err := st.InsertNode(store.Node{
		NodeID: "n-alive", PeerID: "peer-gone", Name: "n-alive", Role: "dev",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "t",
	}); err != nil {
		t.Fatal(err)
	}
	// A pending node whose bind_tty will match a freshly-seen peer.
	if err := st.InsertNode(store.Node{
		NodeID: "n-pending", Name: "n-pending", Role: "dev", BindTTY: "ttys009",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "pending", CreatedAt: "t",
	}); err != nil {
		t.Fatal(err)
	}

	fakePanes := func() (map[string]Pane, error) { return map[string]Pane{}, nil }
	fakePeers := func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "peer-new", TTY: "ttys009"}}, nil
	}

	msg, _ := Tick(fakePanes, fakePeers, st, nil)
	if msg.Err != nil {
		t.Fatalf("unexpected Err: %v", msg.Err)
	}
	if len(msg.Applied.Tombstone) != 1 || msg.Applied.Tombstone[0] != "n-alive" {
		t.Fatalf("Applied.Tombstone = %v, want [n-alive]", msg.Applied.Tombstone)
	}
	if len(msg.Applied.Bind) != 1 || msg.Applied.Bind[0].NodeID != "n-pending" {
		t.Fatalf("Applied.Bind = %v, want [n-pending<-peer-new]", msg.Applied.Bind)
	}

	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]store.Node{}
	for _, n := range nodes {
		byID[n.NodeID] = n
	}
	if got := byID["n-alive"].State; got != "dead" {
		t.Fatalf("n-alive state = %q, want dead (store must actually be written, not just decided)", got)
	}
	if got := byID["n-pending"].State; got != "alive" {
		t.Fatalf("n-pending state = %q, want alive", got)
	}
	if got := byID["n-pending"].PeerID; got != "peer-new" {
		t.Fatalf("n-pending peer_id = %q, want peer-new", got)
	}
}

// THE hard invariant: a failed tmux poll must change nothing in the store,
// even if the broker side would otherwise have produced real reconciliation
// work. A transient failure cascading into a tombstone is the single most
// expensive bug this package can produce.
func TestTickListPanesFailureAppliesNothing(t *testing.T) {
	st := newTickTestStore(t)
	if err := st.InsertNode(store.Node{
		NodeID: "n1", PeerID: "peer-gone", Name: "n1", Role: "dev",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "t",
	}); err != nil {
		t.Fatal(err)
	}

	failingPanes := func() (map[string]Pane, error) { return nil, errors.New("tmux: boom") }
	peersCalled := false
	fakePeers := func() ([]broker.Peer, error) {
		peersCalled = true
		return []broker.Peer{}, nil // peer-gone is indeed gone -- would tombstone n1 if reached
	}

	lastPanes := map[string]Pane{"%1": {ID: "%1", PID: "1"}}
	msg, newLast := Tick(failingPanes, fakePeers, st, lastPanes)

	if msg.Err == nil {
		t.Fatal("expected Err to be set on a failed tmux poll")
	}
	if peersCalled {
		t.Fatal("listPeers must not be called at all once the tmux poll has already failed")
	}
	if len(msg.Applied.Tombstone) != 0 || len(msg.Applied.Bind) != 0 {
		t.Fatalf("Applied must be empty on tmux failure, got %+v", msg.Applied)
	}
	if newLast["%1"].PID != "1" {
		t.Fatal("lastPanes must be unchanged on a failed poll -- there is no new snapshot to advance to")
	}

	nodes, _ := st.ListNodes()
	if nodes[0].State != "alive" {
		t.Fatalf("n1 state = %q, want alive (must be untouched)", nodes[0].State)
	}
}

// Same invariant, broker side: the tmux poll succeeded (so lastPanes SHOULD
// advance -- that signal was real), but the broker failure must still block
// every store write.
func TestTickListPeersFailureAppliesNothingButAdvancesPanes(t *testing.T) {
	st := newTickTestStore(t)
	if err := st.InsertNode(store.Node{
		NodeID: "n1", PeerID: "peer-x", Name: "n1", Role: "dev",
		Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "t",
	}); err != nil {
		t.Fatal(err)
	}

	curPanes := map[string]Pane{"%2": {ID: "%2", PID: "2"}}
	fakePanes := func() (map[string]Pane, error) { return curPanes, nil }
	failingPeers := func() ([]broker.Peer, error) { return nil, errors.New("broker: db locked") }

	msg, newLast := Tick(fakePanes, failingPeers, st, nil)

	if msg.Err == nil {
		t.Fatal("expected Err to be set on a failed broker poll")
	}
	if len(msg.Applied.Tombstone) != 0 || len(msg.Applied.Bind) != 0 {
		t.Fatalf("Applied must be empty on broker failure, got %+v", msg.Applied)
	}
	if newLast["%2"].PID != "2" {
		t.Fatal("lastPanes SHOULD advance on broker failure -- the tmux read was real and independent")
	}

	nodes, _ := st.ListNodes()
	if nodes[0].State != "alive" {
		t.Fatalf("n1 state = %q, want alive (untouched)", nodes[0].State)
	}
}

func TestTickQuietWhenNothingChanged(t *testing.T) {
	st := newTickTestStore(t)
	fakePanes := func() (map[string]Pane, error) { return map[string]Pane{}, nil }
	fakePeers := func() ([]broker.Peer, error) { return []broker.Peer{}, nil }

	msg, _ := Tick(fakePanes, fakePeers, st, nil)
	if msg.Err != nil {
		t.Fatalf("unexpected Err: %v", msg.Err)
	}
	if len(msg.Applied.Tombstone) != 0 || len(msg.Applied.Bind) != 0 {
		t.Fatalf("expected no actions on an empty store/peer set, got %+v", msg.Applied)
	}
}
