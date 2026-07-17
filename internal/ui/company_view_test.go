package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/sync"
)

// The regression this locks: company scoping must never wrap the liveness peer
// source. If it did, a single transient resolution blip that drops an
// in-company peer for one tick would make reconcile tombstone the live node
// forever. WithScope must leave the raw source untouched.
func TestWithScopeDoesNotFilterLivenessSource(t *testing.T) {
	m, _ := liveModel(t)
	called := false
	m.live.listPeers = func() ([]broker.Peer, error) {
		called = true
		return []broker.Peer{{ID: "px", CWD: "/out/of/company"}}, nil
	}
	m = m.WithScope(company.Company{ID: "co-1", Name: "Galaxy"}, "/galaxy/root")

	peers, err := m.live.listPeers()
	if err != nil {
		t.Fatal(err)
	}
	if !called || len(peers) != 1 || peers[0].ID != "px" {
		t.Fatalf("WithScope altered the liveness peer source (got %+v); a scoping blip could now tombstone live agents", peers)
	}
}

// A bound node whose peer is present in the RAW broker must count as alive even
// when the console is scoped to a different company — liveness follows the real
// broker, not the company filter.
func TestBoundNodeStaysAliveUnderForeignScope(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", PeerID: "px",
			Workdir: "/tmp", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	// Scope to a company the peer's cwd does NOT belong to.
	m = m.WithScope(company.Company{ID: "co-1", Name: "Galaxy"}, "/galaxy/root")
	m.live.listPeers = func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "px", CWD: "/somewhere/else", TTY: "ttys9"}}, nil
	}

	m.applyTick(sync.TickMsg{}) // a successful tick

	if m.live.summary.Dead != 0 {
		t.Fatalf("bound node counted dead under a foreign scope (Dead=%d); liveness must follow the raw broker", m.live.summary.Dead)
	}
	if m.live.summary.Alive != 1 {
		t.Fatalf("bound node not alive though its peer is in the raw broker (Alive=%d)", m.live.summary.Alive)
	}
	// The out-of-company peer is bound, so it isn't unmanaged; the scoped
	// unmanaged count must be zero, not leak it.
	if m.live.summary.Unmanaged != 0 {
		t.Fatalf("Unmanaged=%d, want 0", m.live.summary.Unmanaged)
	}
}

func TestHeaderShowsCompanyWhenScoped(t *testing.T) {
	m, _ := liveModel(t)
	m = m.WithScope(company.Company{ID: "co-1", Name: "Acme Corp"}, "/co/root")
	if got := m.header(); !strings.Contains(got, "Acme Corp") {
		t.Fatalf("header should name the scoped company, got %q", got)
	}
}

func TestHeaderOmitsCompanyWhenUnscoped(t *testing.T) {
	m, _ := liveModel(t)
	// No WithScope call: unscoped.
	if got := m.header(); strings.Contains(got, "·") {
		t.Fatalf("unscoped header should not carry a company separator, got %q", got)
	}
}

func TestEmptyStateNamesCompany(t *testing.T) {
	// A live model with no nodes shows the splash; when scoped it must name the
	// company so an empty chart reads as "this company has no agents yet".
	s, err := store.Open(t.TempDir() + "/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	m := NewLive(s, nil).WithScope(company.Company{ID: "co-1", Name: "Beta LLC"}, "/co/root")
	if !strings.Contains(m.View(), "Beta LLC") {
		t.Fatalf("empty-state view should name the scoped company:\n%s", m.View())
	}
}

// WithScope with an empty root must leave the peer source and header unscoped —
// callers pass a resolution through unconditionally, and "no company" must be a
// clean no-op rather than a filter that hides everything.
func TestWithScopeEmptyRootIsUnscoped(t *testing.T) {
	m, _ := liveModel(t)
	m = m.WithScope(company.Company{}, "")
	if got := m.header(); strings.Contains(got, "·") {
		t.Fatalf("empty-root scope should stay unscoped, got header %q", got)
	}
}
