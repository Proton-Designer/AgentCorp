package ui

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
	"github.com/Proton-Designer/AgentCorp/internal/hire"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/sync"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// TickInterval matches claude-peers' own broker poll cadence. Polling faster
// than the substrate updates buys nothing but CPU.
const TickInterval = time.Second

// BrokerDBPath returns the substrate's database. Read-only to us, always.
func BrokerDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude-peers.db")
}

// liveState holds what the tick loop needs between ticks. It lives on the
// Model rather than in a package global so tests can drive it deterministically.
// ActivityWindow is how recently a node must have spoken to read as active.
//
// A parameter, not a magic number baked into the pure layer: "recently" is a
// product judgment, and the pure functions in vitals/ deliberately take it as
// an argument so it can be tuned without touching them.
const ActivityWindow = 60 * time.Second

// ThroughputWindow is the span the sparkline covers, bucketed by minute.
const ThroughputWindow = 10 * time.Minute

type liveState struct {
	st        *store.Store
	lastPanes map[string]sync.Pane
	brokerDB  string
	lastErr   error
	lastSync  time.Time
	stale     bool // true when the most recent poll failed

	// listPeers is the RAW broker peer source — never company-scoped. Both the
	// tick's reconcile feed (which decides life and death, and writes
	// tombstones) and the HUD read from it, so an agent's liveness always
	// reflects the real broker. Scoping is applied only to the unmanaged count,
	// via companyRoot below — see applyTick.
	listPeers func() ([]broker.Peer, error)

	// listMessages and listPanes are the other two live sources. They default to
	// the real broker/tmux, but are injectable so demo mode can drive a populated
	// chart with synthetic activity and no real sessions.
	listMessages func() ([]broker.Message, error)
	listPanes    func() (map[string]sync.Pane, error)

	// company is the resolved company for this launch, for display only. Zero
	// value (empty Name) means the directory is unscoped.
	company company.Company

	// companyRoot scopes the "unmanaged" adoption count to this company. Empty
	// means unscoped. It deliberately does NOT gate liveness.
	companyRoot string

	// unmanaged is the current in-company set of live peers with no node — the
	// candidates the adopt picker offers. Refreshed every tick.
	unmanaged []broker.Peer

	// Last known-good substrate readings. Kept across a failed poll so the
	// HUD can show the last truth rather than zeros — zeros would read as
	// "the company is empty", which is a different and false claim.
	peers   []broker.Peer
	msgs    []broker.Message
	summary vitals.Summary
	spark   string
	ticker  string
	started time.Time

	// hire dependencies, nil in a read-only console. hireWorkdir is where a
	// new agent is spawned (defaults to the operator's cwd).
	hireFlow    *hire.Flow
	hireWorkdir string
}

// tickCmd runs one poll cycle off the UI goroutine and delivers the result
// as a message. Bubble Tea's model update must never block on I/O.
func (m Model) tickCmd() tea.Cmd {
	if m.live == nil || m.live.st == nil {
		return nil
	}
	live := m.live
	return func() tea.Msg {
		msg, next := sync.Tick(
			live.listPanes,
			live.listPeers,
			live.st,
			live.lastPanes,
		)
		// Tick returns the pane snapshot to carry forward; it already declines
		// to advance it when the tmux poll failed.
		live.lastPanes = next
		return msg
	}
}

func scheduleTick() tea.Cmd {
	return tea.Tick(TickInterval, func(time.Time) tea.Msg { return tickWake{} })
}

// tickWake is the timer firing. It's distinct from sync.TickMsg (the poll
// result) so a slow poll can never stack up behind the timer.
type tickWake struct{}

// applyTick folds a completed poll into the model.
//
// The staleness handling is the load-bearing part: a failed poll means we do
// not know the org's state, which is NOT the same as knowing the org is empty.
// sync.Tick already refuses to write on a failed poll; this refuses to *redraw
// as if nothing were wrong*. Silently rendering a stale tree as current is how
// an operator ends up trusting a chart that stopped being true minutes ago.
func (m *Model) applyTick(msg sync.TickMsg) {
	if msg.Err != nil {
		m.live.lastErr = msg.Err
		m.live.stale = true
		return // keep the last known-good tree on screen, marked stale
	}
	m.live.lastErr = nil
	m.live.stale = false
	now := time.Now()
	m.live.lastSync = now

	nodes, err := m.live.st.ListNodes()
	if err != nil {
		m.live.lastErr = err
		m.live.stale = true
		return
	}
	m.rebuild(nodes)

	// Re-read peers and messages for the HUD. These are local read-only SQLite
	// queries against a file the tick just proved reachable.
	//
	// A failure here degrades the HUD but must NOT mark the whole view stale:
	// the tree is still current, and claiming otherwise would be its own lie.
	// We keep the last known-good readings rather than zeroing them — zeros
	// would render as "the company is empty", which is a different false claim
	// than "we don't know".
	if peers, err := m.live.listPeers(); err == nil {
		m.live.peers = peers
	}
	if msgs, err := m.live.listMessages(); err == nil {
		// Scope to this company's traffic: the broker is machine-wide, so
		// unrelated sessions' messages would otherwise inflate the sparkline
		// and ticker with activity that isn't this company's.
		m.live.msgs = scopeMessages(m.live.companyRoot, m.live.peers, msgs)
	}

	m.live.summary = vitals.Vitals(nodes, m.live.peers, m.live.msgs, now, ActivityWindow)
	// Liveness (Alive/Active/Quiet/Dead) is computed from the raw peer list
	// above — correct, and immune to a scoping blip. Only the unmanaged count
	// is scoped to the company, so the HUD doesn't count every other company's
	// sessions as adoptable here. InCompany with an empty root is a no-op.
	m.live.unmanaged = broker.Reconcile(nodes,
		InCompany(m.live.companyRoot, m.live.peers)).Unmanaged
	m.live.summary.Unmanaged = len(m.live.unmanaged)
	if m.live.started.IsZero() {
		m.live.started = now
	}
	m.live.summary.Uptime = now.Sub(m.live.started)
	m.live.spark = sparkline(vitals.Throughput(m.live.msgs, ThroughputWindow, now))
	m.live.ticker = vitals.Ticker(m.live.msgs)
}

// statusOf returns a node's live status glyph input. Nodes are matched to
// store rows by name, which is what the layout tree carries.
func (m Model) statusOf(name string) vitals.Status {
	if m.live == nil {
		return ""
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return ""
	}
	for _, n := range nodes {
		if n.Name == name {
			return vitals.NodeStatus(n, m.live.peers, m.live.msgs,
				time.Now(), ActivityWindow)
		}
	}
	return ""
}

// rebuild swaps in a fresh tree while preserving what the operator was doing:
// which nodes were folded, and which node was selected. Losing either on every
// 1s tick would make the UI unusable — folds would spring open and the cursor
// would jump while you were reading.
func (m *Model) rebuild(nodes []store.Node) {
	collapsed := map[string]bool{}
	for _, n := range m.flat {
		if n.Collapsed {
			collapsed[n.ID] = true
		}
	}
	var selectedID string
	if s := m.selected(); s != nil {
		selectedID = s.ID
	}

	m.root = BuildTree(nodes)
	m.reflatten()

	for _, n := range m.flat {
		if collapsed[n.ID] {
			n.Collapsed = true
		}
	}
	m.reflatten() // folds change visibility, so the flat order changes

	for i, n := range m.flat {
		if n.ID == selectedID {
			m.cursor = i
			break
		}
	}
	if m.cursor >= len(m.flat) {
		m.cursor = max(0, len(m.flat)-1)
	}
}
