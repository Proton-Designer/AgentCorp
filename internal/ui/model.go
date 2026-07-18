package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
	"github.com/Proton-Designer/AgentCorp/internal/hire"
	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/sync"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
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

	// live is nil for a static model (tests, `agentcorp --once`). When set, the
	// model polls the broker and tmux every tick.
	live *liveState

	// Modal input state. Exactly one modal is open at a time (mode is an enum,
	// not a set of flags), so these never conflict.
	mode        mode
	hireInput   input
	msgInput    input
	searchInput input
	filter      string
	confirm     *confirmState
	adoptCursor int // selection index within the unmanaged-peer adopt picker

	// flash is a transient one-line status (last action's outcome). Cleared
	// on the next keypress so it never lingers as stale truth.
	flash string
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
// The hire flow is left nil; call WithHire to enable spawning.
func NewLive(st *store.Store, nodes []store.Node) Model {
	m := New(nodes)
	brokerDB := BrokerDBPath()
	m.live = &liveState{
		st:        st,
		lastPanes: map[string]sync.Pane{},
		brokerDB:  brokerDB,
		// Default: unscoped — every session on the machine. WithScope narrows it.
		listPeers: func() ([]broker.Peer, error) { return broker.ListPeers(brokerDB) },
	}
	return m
}

// WithHire enables the hire action by wiring in an assembled flow and the
// working directory new agents are spawned into. Without this, pressing 'h'
// reports "hire unavailable" rather than silently doing nothing.
func (m Model) WithHire(flow *hire.Flow, workdir string) Model {
	if m.live != nil {
		m.live.hireFlow = flow
		m.live.hireWorkdir = workdir
	}
	return m
}

// WithScope records the company this console is showing: its name titles the
// header, and its root scopes the "unmanaged" adoption list to sessions inside
// the company. Crucially it does NOT scope the peer source used for liveness —
// a bound node lives or dies by the real broker, never by this filter — so a
// transient resolution blip can't tombstone a live agent. An empty companyRoot
// leaves the model unscoped, so callers can pass a resolution through
// unconditionally.
func (m Model) WithScope(c company.Company, companyRoot string) Model {
	if m.live == nil {
		return m
	}
	m.live.company = c
	m.live.companyRoot = companyRoot
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

	case actionResultMsg:
		m.flash = msg.text
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		m.flash = "" // any keypress clears a stale status line

		// A modal, if open, gets first refusal on the key.
		if m.mode != modeNormal {
			nm, cmd, consumed := m.handleModalKey(key)
			if consumed {
				return nm, cmd
			}
			m = nm
		}

		switch key {
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
			if n := m.selected(); n != nil && len(n.Children) > 0 {
				n.Collapsed = !n.Collapsed
				m.reflatten()
			}
		case "h":
			m.openHire()
		case "m":
			m.openMessage()
		case "/":
			m.openSearch()
		case "x":
			m.openFireConfirm()
		case "D":
			m.openDisbandConfirm()
		case "a":
			m.openAdopt()
		}
	}
	return m, nil
}

// renderChart paints the org chart, in colour when live and colour is enabled,
// monochrome otherwise (static models, tests, NO_COLOR).
func (m Model) renderChart() string {
	if m.live == nil {
		return Render(m.root, m.width)
	}
	statuses := m.statusMap()
	return RenderStyled(m.root, m.width, func(id string) vitals.Status {
		return statuses[id]
	})
}

// statusMap classifies every node once per render (keyed by the name the layout
// tree carries), so the styled renderer doesn't re-list the store per card.
func (m Model) statusMap() map[string]vitals.Status {
	out := map[string]vitals.Status{}
	if m.live == nil {
		return out
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return out
	}
	now := time.Now()
	for _, n := range nodes {
		out[n.Name] = vitals.NodeStatus(n, m.live.peers, m.live.msgs, now, ActivityWindow)
	}
	return out
}

// renderAdopt draws the unmanaged-peer picker: each candidate's derived name,
// working directory, and self-reported summary, with the cursor on the current
// selection and the prospective parent named in the header.
func (m Model) renderAdopt() string {
	if m.live == nil {
		return ""
	}
	parent := "(root)"
	if sel := m.selected(); sel != nil {
		parent = sel.ID
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  adopt under %s — ↑↓ select · ⏎ adopt · esc cancel\n", parent))
	for i, p := range m.live.unmanaged {
		marker := "   "
		if i == m.adoptCursor {
			marker = " ▸ "
		}
		line := fmt.Sprintf("%s%-12s  %s", marker, adoptName(p), truncate(p.CWD, 44))
		if p.Summary != "" {
			if room := m.width - len([]rune(line)) - 4; room > 1 {
				line += "  " + truncate(p.Summary, room)
			}
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return b.String()
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

	var b strings.Builder

	if m.root == nil {
		// No agents yet — show the cold-start splash. But a modal (hire) or a
		// flash (the outcome of a hire that just failed/succeeded) must STILL
		// render over it, or pressing h looks like it does nothing.
		b.WriteString(emptyState(m.width, m.companyName()))
	} else {
		b.WriteString(m.header())
		b.WriteString("\n")
		if m.live != nil {
			b.WriteString("  " + hudLine(m.live.summary, m.live.spark, m.live.stale) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(m.renderChart())
		b.WriteString("\n")
	}

	// The activity ticker: the most recent message in the org. Always moving,
	// so the screen has a pulse even when the tree is calm.
	if m.live != nil && m.live.ticker != "" {
		b.WriteString("\n  ◷ " + truncate(m.live.ticker, m.width-6) + "\n")
	}

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

	// A modal, if open, is drawn over the footer area and owns the input line.
	switch m.mode {
	case modeConfirm:
		if m.confirm != nil {
			b.WriteString("\n" + renderConfirm(m.confirm, m.width))
		}
	case modeHire, modeMessage, modeSearch:
		b.WriteString("\n  " + m.activeInput().prompt + " " + m.activeInput().value + "▏\n")
		b.WriteString("  ⏎ confirm · esc cancel\n")
	case modeAdopt:
		b.WriteString("\n" + m.renderAdopt())
	default:
		if n := m.selected(); n != nil {
			b.WriteString(fmt.Sprintf("  selected: %s\n", n.ID))
		}
		if m.flash != "" {
			b.WriteString("  " + m.flash + "\n")
		}
		b.WriteString("  ↑↓ move · space fold · h hire · a adopt · m msg · x fire · shift-D disband · / find · q quit\n")
	}
	return b.String()
}

// activeInput returns the text field for the current modal.
func (m Model) activeInput() input {
	switch m.mode {
	case modeHire:
		return m.hireInput
	case modeMessage:
		return m.msgInput
	case modeSearch:
		return m.searchInput
	}
	return input{}
}

// companyName is the resolved company's display name, or "" when unscoped
// (or on a static, non-live model).
func (m Model) companyName() string {
	if m.live == nil {
		return ""
	}
	return m.live.company.Name
}

// header renders the title bar. The vitals live in hudLine below it. When the
// launch directory is scoped to a company, its name rides in the title so the
// operator can always see which company this console is showing.
func (m Model) header() string {
	title := "╭─ AgentCorp "
	if name := m.companyName(); name != "" {
		title = "╭─ AgentCorp · " + name + " "
	}
	if pad := m.width - len([]rune(title)) - 1; pad > 0 {
		title += strings.Repeat("─", pad) + "╮"
	}
	return title
}

// truncate cuts to n cells, rune-safely.
//
// NOTE (UI-7): this counts runes, not display cells. Correct for the ASCII
// message text it currently handles; wrong the moment a double-width glyph
// appears. Tracked with the rest of the displaywidth work.
func truncate(s string, n int) string {
	if n <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func emptyState(width int, companyName string) string {
	lines := []string{
		"",
		"╭───────────────────────╮",
		"│   A G E N T C O R P   │",
		"╰───────────────────────╯",
		"",
		"a company of agents you can see and steer",
		"",
		"press h to hire your first agent",
		"",
	}
	// When scoped, name the company on the splash so the operator knows the
	// view is already filtered — an empty chart then reads as "this company has
	// no agents yet," not "AgentCorp sees nothing."
	if companyName != "" {
		lines = append(lines, "company: "+companyName, "")
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
