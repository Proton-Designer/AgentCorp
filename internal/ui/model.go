package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aymanmohammed/crew/internal/layout"
	"github.com/aymanmohammed/crew/internal/store"
	"github.com/aymanmohammed/crew/internal/sync"
)

const cardW, cardH = 14, 3

// Model is the Bubble Tea model for the hero screen.
type Model struct {
	root     *layout.Node
	flat     []*layout.Node // depth-first, for cursor movement
	cursor   int
	width    int
	height   int
	quitting bool

	// live is nil for a static model (tests, `crew --once`). When set, the
	// model polls the broker and tmux every tick.
	live *liveState
}

// BuildTree assembles a layout tree from store rows.
//
// Tombstoned nodes are deliberately included — spec §9 requires the row to
// survive so the dim=dead glyph has something to render. Dropping them here
// would silently re-introduce the orphaning that tombstoning exists to prevent.
//
// Returns nil when there is no root, which Render and View both handle.
func BuildTree(nodes []store.Node) *layout.Node {
	byID := make(map[string]*layout.Node, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = &layout.Node{ID: n.Name, W: cardW, H: cardH}
	}

	// Deterministic child order: creation time, then id. Without this, map
	// iteration order would make the chart jump between renders.
	ordered := make([]store.Node, len(nodes))
	copy(ordered, nodes)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt != ordered[j].CreatedAt {
			return ordered[i].CreatedAt < ordered[j].CreatedAt
		}
		return ordered[i].NodeID < ordered[j].NodeID
	})

	var root *layout.Node
	for _, n := range ordered {
		ln := byID[n.NodeID]
		if n.ParentID == "" {
			if root == nil {
				root = ln
			}
			continue
		}
		if p, ok := byID[n.ParentID]; ok {
			p.Children = append(p.Children, ln)
		}
	}
	return root
}

// New builds a static model — no polling. Used by tests and by any caller
// that just wants to render a snapshot.
func New(nodes []store.Node) Model {
	m := Model{root: BuildTree(nodes), width: 80, height: 24}
	m.reflatten()
	return m
}

// NewLive builds a model that polls the broker and tmux every TickInterval.
func NewLive(st *store.Store, nodes []store.Node) Model {
	m := New(nodes)
	m.live = &liveState{
		st:        st,
		lastPanes: map[string]sync.Pane{},
		brokerDB:  BrokerDBPath(),
	}
	return m
}

// reflatten rebuilds the cursor's navigation order. Collapsed subtrees are
// skipped: you cannot select what you cannot see.
func (m *Model) reflatten() {
	m.flat = nil
	if m.root == nil {
		return
	}
	var walk func(*layout.Node)
	walk = func(n *layout.Node) {
		m.flat = append(m.flat, n)
		if n.Collapsed {
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.root)
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) Init() tea.Cmd {
	if m.live == nil {
		return nil
	}
	// Poll immediately so the first frame is current, then start the timer.
	return tea.Batch(m.tickCmd(), scheduleTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tickWake:
		// Timer fired: run a poll and re-arm. Re-arming here rather than
		// after the poll completes keeps the cadence steady even if a poll
		// runs long.
		return m, tea.Batch(m.tickCmd(), scheduleTick())

	case sync.TickMsg:
		m.applyTick(msg)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "down", "j":
			if m.cursor < len(m.flat)-1 {
				m.cursor++
			}

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case " ":
			// Fold only makes sense on a node with children.
			if n := m.selected(); n != nil && len(n.Children) > 0 {
				n.Collapsed = !n.Collapsed
				m.reflatten()
			}
		}
	}
	return m, nil
}

func (m Model) selected() *layout.Node {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return nil
	}
	return m.flat[m.cursor]
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.root == nil {
		return emptyState(m.width)
	}

	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(Render(m.root, m.width))
	b.WriteString("\n\n")

	// A stale view must say so. The tree on screen is the last thing we knew
	// to be true, not what is true now — and an operator acting on a silently
	// stale org chart is exactly the harm this whole design tries to avoid.
	if m.live != nil && m.live.stale {
		b.WriteString("  ⚠ STALE — poll failed; showing last known state")
		if m.live.lastErr != nil {
			b.WriteString(": " + m.live.lastErr.Error())
		}
		b.WriteString("\n")
	}

	if n := m.selected(); n != nil {
		fold := ""
		if len(n.Children) > 0 {
			fold = " · space to fold"
			if n.Collapsed {
				fold = " · space to unfold"
			}
		}
		b.WriteString(fmt.Sprintf("  selected: %s%s\n", n.ID, fold))
	}
	b.WriteString("  ↑↓ move · space fold · q quit\n")
	return b.String()
}

// header renders the vitals strip. Live status reads "live" only when the
// most recent poll actually succeeded — never optimistically.
func (m Model) header() string {
	status := ""
	if m.live != nil {
		if m.live.stale {
			status = "⚠ stale "
		} else {
			status = "● live "
		}
	}
	title := fmt.Sprintf("╭─ CREW ─ %s%d agents ", status, len(m.flat))
	if pad := m.width - len([]rune(title)) - 1; pad > 0 {
		title += strings.Repeat("─", pad) + "╮"
	}
	return title
}

func emptyState(width int) string {
	lines := []string{
		"",
		"   ██████ ██████  ███████ ██     ██",
		"  ██      ██   ██ ██      ██     ██",
		"  ██      ██████  █████   ██  █  ██",
		"  ██      ██   ██ ██      ██ ███ ██",
		"   ██████ ██   ██ ███████  ███ ███ ",
		"",
		"  a company of agents you can see and steer",
		"",
		"  press h to hire your first agent",
		"",
	}
	var b strings.Builder
	for _, l := range lines {
		pad := 0
		if width > len([]rune(l)) {
			pad = (width - len([]rune(l))) / 2
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}
