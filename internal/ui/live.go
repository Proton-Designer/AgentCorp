package ui

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/store"
	"github.com/aymanmohammed/crew/internal/sync"
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
type liveState struct {
	st        *store.Store
	lastPanes map[string]sync.Pane
	brokerDB  string
	lastErr   error
	lastSync  time.Time
	stale     bool // true when the most recent poll failed
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
			sync.ListPanes,
			func() ([]broker.Peer, error) { return broker.ListPeers(live.brokerDB) },
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
	m.live.lastSync = time.Now()

	nodes, err := m.live.st.ListNodes()
	if err != nil {
		m.live.lastErr = err
		m.live.stale = true
		return
	}
	m.rebuild(nodes)
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
