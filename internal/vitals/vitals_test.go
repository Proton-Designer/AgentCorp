package vitals

import (
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func vnode(id, peerID, parentID, state, createdAt string) store.Node {
	return store.Node{
		NodeID: id, PeerID: peerID, ParentID: parentID,
		Name: id, Role: "dev", Workdir: "/tmp", SpawnMode: "tmux-window",
		State: state, CreatedAt: createdAt,
	}
}

func vpeer(id, tty string) broker.Peer {
	return broker.Peer{ID: id, PID: 1, CWD: "/tmp", TTY: tty, RegisteredAt: "t", LastSeen: "t"}
}

func vmsg(fromID, sentAt string) broker.Message {
	return broker.Message{ID: 1, FromID: fromID, ToID: "someone", Text: "x", SentAt: sentAt}
}

var fixedNow = mustParse("2026-07-16T12:00:00Z")

const testWindow = 60 * time.Second

func mustParse(s string) time.Time {
	t, ok := parseTimestamp(s)
	if !ok {
		panic("bad fixture timestamp: " + s)
	}
	return t
}

func TestVitalsEmptyInputsProduceZeroSummary(t *testing.T) {
	got := Vitals(nil, nil, nil, fixedNow, testWindow)
	want := Summary{}
	if got != want {
		t.Fatalf("Vitals(nil, nil, nil) = %+v, want %+v", got, want)
	}
}

func TestVitalsCountsActiveForRecentSender(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peers := []broker.Peer{vpeer("p1", "ttys000")}
	msgs := []broker.Message{vmsg("p1", "2026-07-16T11:59:30Z")} // 30s before fixedNow

	got := Vitals(nodes, peers, msgs, fixedNow, testWindow)
	if got.Active != 1 || got.Quiet != 0 {
		t.Fatalf("Active=%d Quiet=%d, want Active=1 Quiet=0", got.Active, got.Quiet)
	}
	if got.Alive != 1 {
		t.Fatalf("Alive = %d, want 1", got.Alive)
	}
}

func TestVitalsCountsQuietForStaleSender(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peers := []broker.Peer{vpeer("p1", "ttys000")}
	msgs := []broker.Message{vmsg("p1", "2026-07-16T11:00:00Z")} // 1h before fixedNow, outside window

	got := Vitals(nodes, peers, msgs, fixedNow, testWindow)
	if got.Quiet != 1 || got.Active != 0 {
		t.Fatalf("Active=%d Quiet=%d, want Active=0 Quiet=1", got.Active, got.Quiet)
	}
}

func TestVitalsCountsQuietForNodeThatNeverSpoke(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peers := []broker.Peer{vpeer("p1", "ttys000")}

	got := Vitals(nodes, peers, nil, fixedNow, testWindow)
	if got.Quiet != 1 || got.Active != 0 {
		t.Fatalf("Active=%d Quiet=%d, want Active=0 Quiet=1 (no messages at all)", got.Active, got.Quiet)
	}
}

// A message TO a peer must never count as that peer's own activity — only
// messages the peer itself sent. Otherwise a quiet node "goes active" just
// because someone else pinged it.
func TestVitalsIgnoresMessagesReceivedNotSent(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peers := []broker.Peer{vpeer("p1", "ttys000")}
	msgs := []broker.Message{{ID: 1, FromID: "someone-else", ToID: "p1", Text: "x", SentAt: "2026-07-16T11:59:59Z"}}

	got := Vitals(nodes, peers, msgs, fixedNow, testWindow)
	if got.Active != 0 || got.Quiet != 1 {
		t.Fatalf("Active=%d Quiet=%d, want Active=0 Quiet=1 (p1 never sent anything)", got.Active, got.Quiet)
	}
}

func TestVitalsCountsTombstonedNodeAsDead(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "dead", "2026-07-16T00:00:00Z")}
	got := Vitals(nodes, nil, nil, fixedNow, testWindow)
	if got.Dead != 1 {
		t.Fatalf("Dead = %d, want 1", got.Dead)
	}
	if got.Alive != 0 {
		t.Fatalf("Alive = %d, want 0", got.Alive)
	}
}

// A node still recorded as state='alive' whose peer has vanished from the
// broker (not yet reconciled to 'dead' in the DB) must count as Dead in a
// live snapshot — trusting the stale state field would show a peer as alive
// after its process is gone.
func TestVitalsCountsBoundButPeerGoneAsDead(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	got := Vitals(nodes, nil, nil, fixedNow, testWindow) // no peers: p1 is gone
	if got.Dead != 1 {
		t.Fatalf("Dead = %d, want 1 (stale-alive node whose peer vanished)", got.Dead)
	}
	if got.Alive != 0 {
		t.Fatalf("Alive = %d, want 0", got.Alive)
	}
}

func TestVitalsDoesNotCountPendingOrFailedNodes(t *testing.T) {
	nodes := []store.Node{
		vnode("pending1", "", "", "pending", "2026-07-16T00:00:00Z"),
		vnode("failed1", "", "", "failed", "2026-07-16T00:00:00Z"),
	}
	got := Vitals(nodes, nil, nil, fixedNow, testWindow)
	if got.Alive != 0 || got.Dead != 0 || got.Unmanaged != 0 {
		t.Fatalf("Summary = %+v, want all-zero (pending/failed nodes counted in no bucket)", got)
	}
}

func TestVitalsUnmanagedExcludesPeerClaimedByPendingBind(t *testing.T) {
	nodes := []store.Node{vnode("n1", "", "", "pending", "2026-07-16T00:00:00Z")}
	nodes[0].BindTTY = "/dev/ttys024"
	peers := []broker.Peer{vpeer("p1", "ttys024")} // matches n1's bind_tty after normalization

	got := Vitals(nodes, peers, nil, fixedNow, testWindow)
	if got.Unmanaged != 0 {
		t.Fatalf("Unmanaged = %d, want 0 (p1 is claimed by a pending bind, not free-floating)", got.Unmanaged)
	}
}

func TestVitalsUnmanagedCountsTrulyFreePeers(t *testing.T) {
	peers := []broker.Peer{vpeer("p1", "ttys000"), vpeer("p2", "ttys001")}
	got := Vitals(nil, peers, nil, fixedNow, testWindow)
	if got.Unmanaged != 2 {
		t.Fatalf("Unmanaged = %d, want 2", got.Unmanaged)
	}
}

func TestVitalsUptimeIsSinceEarliestNodeCreatedAt(t *testing.T) {
	nodes := []store.Node{
		vnode("n1", "", "", "pending", "2026-07-16T10:00:00Z"),
		vnode("n2", "", "", "pending", "2026-07-16T08:00:00Z"), // earlier
		vnode("n3", "", "", "pending", "2026-07-16T11:00:00Z"),
	}
	got := Vitals(nodes, nil, nil, fixedNow, testWindow) // fixedNow = 2026-07-16T12:00:00Z
	want := 4 * time.Hour                                // 12:00 - 08:00
	if got.Uptime != want {
		t.Fatalf("Uptime = %v, want %v", got.Uptime, want)
	}
}

func TestVitalsUptimeZeroWithNoNodes(t *testing.T) {
	got := Vitals(nil, nil, nil, fixedNow, testWindow)
	if got.Uptime != 0 {
		t.Fatalf("Uptime = %v, want 0", got.Uptime)
	}
}

func TestVitalsUptimeSkipsUnparseableCreatedAt(t *testing.T) {
	nodes := []store.Node{
		vnode("bad", "", "", "pending", "not-a-timestamp"),
		vnode("good", "", "", "pending", "2026-07-16T10:00:00Z"),
	}
	got := Vitals(nodes, nil, nil, fixedNow, testWindow)
	want := 2 * time.Hour // 12:00 - 10:00, "bad" ignored rather than crashing or winning as earliest
	if got.Uptime != want {
		t.Fatalf("Uptime = %v, want %v", got.Uptime, want)
	}
}

func TestVitalsAliveEqualsActivePlusQuiet(t *testing.T) {
	nodes := []store.Node{
		vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z"),
		vnode("n2", "p2", "", "alive", "2026-07-16T00:00:00Z"),
	}
	peers := []broker.Peer{vpeer("p1", "ttys000"), vpeer("p2", "ttys001")}
	msgs := []broker.Message{vmsg("p1", "2026-07-16T11:59:50Z")} // only p1 recently active

	got := Vitals(nodes, peers, msgs, fixedNow, testWindow)
	if got.Alive != got.Active+got.Quiet {
		t.Fatalf("Alive=%d != Active(%d)+Quiet(%d)", got.Alive, got.Active, got.Quiet)
	}
}

// Regression guard for the rejected heuristic (spec §5.2): a peer's
// free-text summary must never influence Active/Quiet, no matter how
// suggestive it reads. Only message recency may. This replaces the earlier
// "Working/Idle/Blocked must stay zero" canary now that those fields are
// gone entirely — the real rule survives as "never derive activity from
// summary text," not as a placeholder for undecided fields.
func TestVitalsNeverDerivesActivityFromSummaryText(t *testing.T) {
	nodes := []store.Node{vnode("n1", "p1", "", "alive", "2026-07-16T00:00:00Z")}
	peer := vpeer("p1", "ttys000")
	peer.Summary = "actively working right now, mid-tool-call, definitely busy"
	peers := []broker.Peer{peer}

	// No messages at all: per message-recency rules this must be Quiet,
	// regardless of how emphatically the summary claims otherwise.
	got := Vitals(nodes, peers, nil, fixedNow, testWindow)
	if got.Active != 0 || got.Quiet != 1 {
		t.Fatalf("Active=%d Quiet=%d, want Active=0 Quiet=1 — summary text must never make a silent node Active",
			got.Active, got.Quiet)
	}
}
