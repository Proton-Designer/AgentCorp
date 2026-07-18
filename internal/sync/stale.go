package sync

import (
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// PendingGrace is the outer bound on a pending hire. A hire waits up to the
// hire flow's bind timeout synchronously; after that it stays pending so the
// tick can bind it if the session was merely slow. But a session that spawned
// and NEVER registers — crashed, bad workdir, auth failure — must not sit
// pending (and orphan its tmux pane) forever. Once a still-unbound pending node
// is older than this, the tick fails it. Comfortably larger than the bind
// timeout (90s) so a genuinely-slow-but-fine session is never failed early.
const PendingGrace = 5 * time.Minute

// StalePending returns the ids of pending, still-unbound nodes whose age
// exceeds grace — hires that spawned but never registered. Pure: (nodes, now,
// grace) in, ids out, no I/O. A node with an unparseable created_at is left
// alone (we can't prove it's stale), and a node about to be bound this tick is
// the caller's job to exclude (it registered, just slowly).
func StalePending(nodes []store.Node, now time.Time, grace time.Duration) []string {
	var out []string
	for _, n := range nodes {
		if n.State != "pending" || n.PeerID != "" {
			continue
		}
		t, ok := parseNodeTime(n.CreatedAt)
		if !ok {
			continue
		}
		if now.Sub(t) >= grace {
			out = append(out, n.NodeID)
		}
	}
	return out
}

// parseNodeTime parses a node's created_at, which the hire flow writes as
// RFC3339. Returns ok=false for anything it can't parse (e.g. the bare "1"
// timestamps some tests use), so unparseable ages never trigger a fail.
func parseNodeTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
