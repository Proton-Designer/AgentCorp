// Command agentcorp renders a company of AI agents as a live, operable org chart.
//
// A company is scoped to a directory subtree: the directory AgentCorp launches
// in is resolved (walking up, git-style) to the nearest .agentcorp/company.toml.
// If one is found, only sessions inside that company are shown and its hierarchy
// lives in that folder; if none is found, AgentCorp offers to create one, or
// runs unscoped (every session on the machine) if the operator declines.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
	"github.com/Proton-Designer/AgentCorp/internal/hire"
	"github.com/Proton-Designer/AgentCorp/internal/spawn"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agentcorp:", err)
		os.Exit(1)
	}
}

func run() error {
	for _, a := range os.Args[1:] {
		if a == "--demo" {
			return runDemo()
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// The global config dir holds machine-wide state: the channel consent
	// record, and the fallback store used when a directory isn't scoped to any
	// company.
	globalDir := filepath.Join(home, ".config", "agentcorp")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		return err
	}

	cwd, _ := os.Getwd()

	// One reader for every plain-terminal prompt below. Sharing it matters:
	// bufio.Reader buffers ahead, so two separate readers on os.Stdin could make
	// the second one miss input the first already pulled into its buffer.
	stdin := bufio.NewReader(os.Stdin)

	// First-run consent must happen BEFORE the alt-screen takes over the
	// terminal — it needs plain stdin/stdout so the human can actually read the
	// warning and answer it (spec §6.2 / SE-5). Consent is machine-wide, not
	// per-company, so it stays in the global dir.
	consentPath := filepath.Join(globalDir, "consent")
	if err := ensureConsent(stdin, consentPath); err != nil {
		return err
	}

	// Resolve (or create) the company for this directory. This decides both what
	// the operator sees and where the hierarchy is stored.
	co, root, err := resolveCompany(stdin, cwd)
	if err != nil {
		return err
	}

	// A scoped company keeps its store inside the folder, so one company's org
	// never bleeds into another's. Unscoped launches use the global store.
	storePath := filepath.Join(globalDir, "agentcorp.db")
	if root != "" {
		storePath = company.StorePath(root)
	}

	s, err := store.Open(storePath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	nodes, err := s.ListNodes()
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	// Peers are read RAW (never company-scoped) for anything that decides
	// liveness or binding: a node we own must live or die by the real broker,
	// and a fresh spawn must be bindable the instant it registers. Company
	// scoping is a display concern applied later, only to the unmanaged count
	// (see ui.WithScope / applyTick). The per-company store already scopes which
	// nodes exist, so the chart itself shows only this company's agents.
	rawPeers := func() ([]broker.Peer, error) { return broker.ListPeers(ui.BrokerDBPath()) }

	// Assemble the hire flow. The tmux socket is empty so AgentCorp uses the
	// operator's own tmux server — the console and the agents share it.
	flow := &hire.Flow{
		Store:       s,
		Adapter:     spawn.NewTmuxWindowAdapter(""),
		Gates:       hire.NewGateClearer(""),
		ListPeers:   rawPeers,
		IDFunc:      func() string { return "n-" + time.Now().UTC().Format("20060102T150405.000000") },
		ConsentPath: consentPath,
		// A freshly spawned claude cold-starts every configured MCP server
		// (npx/bun subprocesses, network handshakes) before it registers with
		// the broker. On a machine with many MCP servers that can take well over
		// 30s, so the bind wait is generous — a hire that times out a few seconds
		// early is worse than one that waits a little longer. Overridable so a
		// slower or faster setup can tune it without a rebuild.
		BindTimeout: bindTimeout(90 * time.Second),
		BindPoll:    250 * time.Millisecond,
	}

	model := ui.NewLive(s, nodes).
		WithScope(co, root).
		WithHire(flow, cwd)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

// bindTimeout returns how long a hire waits for a spawned session to register,
// defaulting to def but overridable via AGENTCORP_BIND_TIMEOUT (a Go duration
// like "120s" or "2m"). An unset or unparseable value falls back to def rather
// than failing to start.
func bindTimeout(def time.Duration) time.Duration {
	if v := os.Getenv("AGENTCORP_BIND_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// runDemo launches AgentCorp against a seeded, synthetic org — no consent, no
// company prompt, no real broker/tmux, no spawned Claude sessions. It exists so
// the console can be shown, driven, and screenshotted (and a terminal-automation
// harness can test its settle logic against the live ~1s repaint) without any
// machine side effects.
func runDemo() error {
	dir, err := os.MkdirTemp("", "agentcorp-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	s, err := store.Open(filepath.Join(dir, "demo.db"))
	if err != nil {
		return fmt.Errorf("demo store: %w", err)
	}
	defer s.Close()

	// Seed a small org: a lead with three reports and a grand-report. Alive and
	// bound to synthetic peers; empty spawn_ref so nothing tries to reap them.
	seed := []store.Node{
		{NodeID: "1", Name: "CEO", Role: "lead", State: "alive", PeerID: "demo-1", Workdir: dir, SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:00Z"},
		{NodeID: "2", Name: "backend", Role: "engineer", ParentID: "1", State: "alive", PeerID: "demo-2", Workdir: dir, SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:01Z"},
		{NodeID: "3", Name: "frontend", Role: "engineer", ParentID: "1", State: "alive", PeerID: "demo-3", Workdir: dir, SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:02Z"},
		{NodeID: "4", Name: "research", Role: "researcher", ParentID: "1", State: "alive", PeerID: "demo-4", Workdir: dir, SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:03Z"},
		{NodeID: "5", Name: "intern", Role: "engineer", ParentID: "2", State: "alive", PeerID: "demo-5", Workdir: dir, SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:04Z"},
	}
	peerIDs := make([]string, len(seed))
	for i, n := range seed {
		if err := s.InsertNode(n); err != nil {
			return fmt.Errorf("seed node %s: %w", n.Name, err)
		}
		peerIDs[i] = n.PeerID
	}
	nodes, err := s.ListNodes()
	if err != nil {
		return err
	}

	// Injected sources: fixed live peers (nodes stay alive) and a message stream
	// that GROWS over real time, so the sparkline and ticker animate on every
	// tick — the periodic-repaint case a settle detector has to handle.
	peers := demoPeers(peerIDs)
	start := time.Now()
	msgs := demoMessages(start, peerIDs)

	model := ui.NewDemo(s, nodes, peers, msgs, "Demo Co")
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

// demoSummaries gives each seeded agent a distinct, plausible self-summary so the
// speech-bubble / nameplate layer has real (if synthetic) text to show. Indexed
// to match the seed order [CEO, backend, frontend, research, intern].
var demoSummaries = []string{
	"steering the Q3 roadmap; unblocking the team",
	"refactoring the payments ledger for idempotency",
	"polishing the dashboard's live animations",
	"benchmarking retrieval latency across shards",
	"writing the backend's missing edge-case tests",
}

func demoPeers(ids []string) func() ([]broker.Peer, error) {
	ps := make([]broker.Peer, len(ids))
	for i, id := range ids {
		summary := "working on the demo"
		if i < len(demoSummaries) {
			summary = demoSummaries[i]
		}
		ps[i] = broker.Peer{ID: id, CWD: "/demo", Summary: summary}
	}
	return func() ([]broker.Peer, error) { return ps, nil }
}

var demoTexts = []string{
	"on it", "shipped the fix", "reviewing now", "found an edge case",
	"tests green", "need a second pair of eyes", "deploying", "done",
	"pushing to staging", "can you take a look?", "rebased", "LGTM",
}

// demoStep is the synthetic activity cadence. Fast enough that a message is
// almost always fresh (a particle in flight for F1), slow enough to read.
const demoStep = 1800 * time.Millisecond

// demoBackfill seeds this many messages in the recent past so the org is alive
// the instant the demo opens — every reporting line has spoken within the
// activity window, so nodes breathe and the ticker/sparkline are populated from
// frame one rather than warming up over the first minute.
const demoBackfill = 24

// demoMessages returns a message source that follows the REPORTING LINES (not a
// round-robin), so the traffic maps onto real tree edges the message-flow overlay
// can animate. The stream is backfilled into the recent past and then grows with
// elapsed time, alternating direction (a report replying up its manager line).
func demoMessages(start time.Time, peerIDs []string) func() ([]broker.Message, error) {
	// Parent→child pairs matching the seeded hierarchy: CEO↔backend, CEO↔frontend,
	// CEO↔research, backend↔intern.
	type edge struct{ a, b string }
	edges := []edge{
		{peerIDs[0], peerIDs[1]},
		{peerIDs[0], peerIDs[2]},
		{peerIDs[0], peerIDs[3]},
		{peerIDs[1], peerIDs[4]},
	}
	base := start.Add(-demoBackfill * demoStep)
	return func() ([]broker.Message, error) {
		// +1 so the newest message lands at ~now (age < demoStep), keeping a fresh
		// pulse inside FlowWindow at all times rather than trailing a step behind.
		total := demoBackfill + int(time.Since(start)/demoStep) + 1
		out := make([]broker.Message, 0, total)
		for i := 0; i < total; i++ {
			e := edges[i%len(edges)]
			from, to := e.a, e.b
			if i%2 == 1 { // odd steps reply back up the line
				from, to = e.b, e.a
			}
			out = append(out, broker.Message{
				ID:     int64(i),
				FromID: from,
				ToID:   to,
				Text:   demoTexts[i%len(demoTexts)],
				SentAt: base.Add(time.Duration(i) * demoStep).UTC().Format(time.RFC3339Nano),
			})
		}
		return out, nil
	}
}

// sanitizeName strips ANSI escape sequences and control characters from a
// plain-terminal line read, leaving only printable text (trimmed). So a
// Down-arrow (ESC [ B) or a lone Escape typed into the company prompt is
// discarded rather than embedded in the name.
func sanitizeName(s string) string {
	var b strings.Builder
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		if r[i] == 0x1b { // ESC — drop the whole escape sequence
			if i+1 < len(r) && r[i+1] == '[' {
				i += 2
				for i < len(r) && !(r[i] >= '@' && r[i] <= '~') { // CSI final byte
					i++
				}
			}
			continue
		}
		if r[i] < 0x20 || r[i] == 0x7f { // other control chars
			continue
		}
		b.WriteRune(r[i])
	}
	return strings.TrimSpace(b.String())
}

// resolveCompany finds the company that owns cwd, or offers to create one.
//
// Returns the resolved company and its canonical root. A root of "" means the
// operator chose to run unscoped — every session on the machine is visible and
// the global store is used. A malformed company definition is surfaced as an
// error rather than silently ignored.
func resolveCompany(stdin *bufio.Reader, cwd string) (company.Company, string, error) {
	res, err := company.Resolve(cwd)
	if err != nil {
		return company.Company{}, "", err
	}
	if res.Found {
		fmt.Printf("  Company: %s\n", res.Company.Name)
		time.Sleep(300 * time.Millisecond)
		return res.Company, res.Root, nil
	}

	fmt.Println()
	fmt.Println("  This directory isn't linked to a company yet.")
	fmt.Print("  Name one to scope this folder (or leave blank to run unscoped): ")
	line, _ := stdin.ReadString('\n')
	// Sanitize before use. This is a plain line read, not a TUI input, so a user
	// (or an automated driver) who presses arrows/Escape to navigate has those
	// escape sequences land in the line as literal bytes (observed: a driver
	// left "t^[[Bi^[l" in the field). Strip escape sequences and control chars so
	// stray navigation keys can't corrupt a company name — worst case the name
	// comes out empty and we run unscoped, which is the safe default.
	name := sanitizeName(line)
	if name == "" {
		fmt.Println("  Running unscoped — every Claude session on this machine is visible.")
		time.Sleep(400 * time.Millisecond)
		return company.Company{}, "", nil
	}

	id := "co-" + time.Now().UTC().Format("20060102T150405.000000")
	if _, err := company.Create(cwd, name, id); err != nil {
		return company.Company{}, "", fmt.Errorf("create company: %w", err)
	}
	// Re-resolve so the root we hand back is the canonical one the peer filter
	// compares against.
	res, err = company.Resolve(cwd)
	if err != nil || !res.Found {
		return company.Company{}, "", fmt.Errorf("company created but could not be re-read: %v", err)
	}
	fmt.Printf("  Created company %q — linked to %s\n", res.Company.Name, filepath.Join(cwd, company.ConfigDir))
	time.Sleep(500 * time.Millisecond)
	return res.Company, res.Root, nil
}

// ensureConsent shows the first-run consent screen if it hasn't been granted.
//
// Deliberately on plain stdin/stdout, before the TUI: automating a security
// prompt on the operator's behalf is only honest if they granted it somewhere
// they could actually read what they were agreeing to.
func ensureConsent(stdin *bufio.Reader, path string) error {
	if hire.HasConsent(path) {
		return nil
	}
	fmt.Print(hire.ConsentText)
	fmt.Print("  Type 'yes' to grant, anything else to quit: ")

	line, _ := stdin.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(line)) != "yes" {
		return fmt.Errorf("consent not granted; not starting")
	}
	if err := hire.RecordConsent(path); err != nil {
		return fmt.Errorf("record consent: %w", err)
	}
	fmt.Println("  Granted. Starting AgentCorp...")
	time.Sleep(400 * time.Millisecond)
	return nil
}
