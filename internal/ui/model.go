package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aymanmohammed/crew/internal/layout"
	"github.com/aymanmohammed/crew/internal/store"
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

func New(nodes []store.Node) Model {
	m := Model{root: BuildTree(nodes), width: 80, height: 24}
	m.reflatten()
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

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

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
	b.WriteString(header(m.width, len(m.flat)))
	b.WriteString("\n\n")
	b.WriteString(Render(m.root, m.width))
	b.WriteString("\n\n")

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

func header(width, n int) string {
	title := fmt.Sprintf("╭─ CREW ─ %d agents ", n)
	if pad := width - len([]rune(title)) - 1; pad > 0 {
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
