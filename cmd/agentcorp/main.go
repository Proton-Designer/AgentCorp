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

	// The peer source is scoped to the company (unscoped root == "" passes every
	// peer through). The hire flow and the live model share the same source so
	// scoping is applied identically everywhere a peer is read.
	rawPeers := func() ([]broker.Peer, error) { return broker.ListPeers(ui.BrokerDBPath()) }
	scopedPeers := ui.ScopedPeers(root, rawPeers)

	// Assemble the hire flow. The tmux socket is empty so AgentCorp uses the
	// operator's own tmux server — the console and the agents share it.
	flow := &hire.Flow{
		Store:       s,
		Adapter:     spawn.NewTmuxWindowAdapter(""),
		Gates:       hire.NewGateClearer(""),
		ListPeers:   scopedPeers,
		IDFunc:      func() string { return "n-" + time.Now().UTC().Format("20060102T150405.000000") },
		ConsentPath: consentPath,
		BindTimeout: 30 * time.Second,
		BindPoll:    250 * time.Millisecond,
	}

	model := ui.NewLive(s, nodes).
		WithScope(co, root).
		WithHire(flow, cwd)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
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
	name := strings.TrimSpace(line)
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
