// Command crew renders a company of AI agents as a live, operable org chart.
//
// Phase 1: reads the sidecar store and draws the tree. Broker sync and the
// hire/fire lifecycle land in Phases 2 and 3.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/hire"
	"github.com/aymanmohammed/crew/internal/spawn"
	"github.com/aymanmohammed/crew/internal/store"
	"github.com/aymanmohammed/crew/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "crew:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "crew")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	s, err := store.Open(filepath.Join(dir, "crew.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	nodes, err := s.ListNodes()
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	// First-run consent must happen BEFORE the alt-screen takes over the
	// terminal — it needs plain stdin/stdout so the human can actually read
	// the warning and answer it (spec §6.2 / SE-5).
	consentPath := filepath.Join(dir, "consent")
	if err := ensureConsent(consentPath); err != nil {
		return err
	}

	// Assemble the hire flow. The tmux socket is empty so CREW uses the
	// operator's own tmux server — the console and the agents share it.
	cwd, _ := os.Getwd()
	flow := &hire.Flow{
		Store:       s,
		Adapter:     spawn.NewTmuxWindowAdapter(""),
		Gates:       hire.NewGateClearer(""),
		ListPeers:   func() ([]broker.Peer, error) { return broker.ListPeers(ui.BrokerDBPath()) },
		IDFunc:      func() string { return "n-" + time.Now().UTC().Format("20060102T150405.000000") },
		ConsentPath: consentPath,
		BindTimeout: 30 * time.Second,
		BindPoll:    250 * time.Millisecond,
	}

	model := ui.NewLive(s, nodes).WithHire(flow, cwd)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

// ensureConsent shows the first-run consent screen if it hasn't been granted.
//
// Deliberately on plain stdin/stdout, before the TUI: automating a security
// prompt on the operator's behalf is only honest if they granted it somewhere
// they could actually read what they were agreeing to.
func ensureConsent(path string) error {
	if hire.HasConsent(path) {
		return nil
	}
	fmt.Print(hire.ConsentText)
	fmt.Print("  Type 'yes' to grant, anything else to quit: ")

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(strings.ToLower(line)) != "yes" {
		return fmt.Errorf("consent not granted; not starting")
	}
	if err := hire.RecordConsent(path); err != nil {
		return fmt.Errorf("record consent: %w", err)
	}
	fmt.Println("  Granted. Starting CREW...")
	time.Sleep(400 * time.Millisecond)
	return nil
}
