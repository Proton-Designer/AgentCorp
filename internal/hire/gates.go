// Package hire orchestrates the full hire flow: create a pending node, spawn a
// session, clear the startup gates, and bind the resulting peer.
//
// It is the seam where CREW's fiction (a node in our store) meets the mesh (a
// real claude session registered with the broker). Everything it does is a
// real process action PLUS a metadata edit — spec §4.
package hire

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Gate is a startup prompt a freshly spawned session blocks on. A session does
// not register with the broker until every gate is cleared, so an unattended
// hire hangs forever without this.
//
// Verified empirically (spec §6.2, n=2 on real spawns): there are TWO, with
// different scopes, and no flag suppresses either. That research is settled —
// `claude --help` (v2.1.212) doesn't even list the channels flag, and both
// live doc pages describe the dialog as the only path. send-keys is not a
// stopgap; it is the only path.
type Gate struct {
	Name string
	// Match is matched against the pane's visible text. Deliberately a
	// substring of the prompt's own wording rather than an exact frame:
	// the surrounding chrome (box drawing, wrapping, colour) varies with
	// terminal width, and an exact match would break on a resize.
	Match string
	// Keys are sent only after Match is seen. Never blind — sending Enter
	// into a pane that isn't showing a prompt would type it into whatever
	// IS there, which for a live agent means submitting an empty turn.
	Keys []string
	// Optional gates may legitimately never appear.
	Optional bool
}

// DefaultGates are the two prompts a real spawn hits, in the order they fire.
//
// Order matters: workspace-trust fires first and only in a directory the user
// hasn't approved before, so hiring into a fresh workdir hits both while
// hiring into a familiar one hits only the second.
var DefaultGates = []Gate{
	{
		Name:     "workspace-trust",
		Match:    "Do you trust the files in this folder?",
		Keys:     []string{"Enter"},
		Optional: true, // remembered per-directory; absent in a known workdir
	},
	{
		Name:  "channel-consent",
		Match: "Loading development channels",
		Keys:  []string{"Enter"},
		// NOT optional: fires on EVERY session regardless of directory
		// trust. Measured in a directory where two sessions had been live
		// 40+ minutes. This is why gate clearing is a permanent hire step
		// rather than a first-run footnote.
	},
}

// paneCapturer reads a pane's visible text. Injectable so gate logic is
// testable without a real tmux server or a real claude session.
type paneCapturer func(ctx context.Context, paneID string) (string, error)

// keySender types into a pane.
type keySender func(ctx context.Context, paneID string, keys []string) error

// GateClearer clears startup gates on a freshly spawned pane.
type GateClearer struct {
	Gates    []Gate
	Capture  paneCapturer
	SendKeys keySender

	// Timeout bounds the whole sequence. A gate that never appears when it
	// is required means the session is stuck somewhere we don't understand,
	// and hanging forever is worse than failing loudly.
	Timeout time.Duration
	// Poll is how often the pane is read while waiting.
	Poll time.Duration
}

func NewGateClearer(socket string) *GateClearer {
	return &GateClearer{
		Gates:   DefaultGates,
		Timeout: 30 * time.Second,
		Poll:    150 * time.Millisecond,
		Capture: func(ctx context.Context, paneID string) (string, error) {
			args := []string{}
			if socket != "" {
				args = append(args, "-L", socket)
			}
			args = append(args, "capture-pane", "-p", "-t", paneID)
			out, err := exec.CommandContext(ctx, "tmux", args...).Output()
			return string(out), err
		},
		SendKeys: func(ctx context.Context, paneID string, keys []string) error {
			args := []string{}
			if socket != "" {
				args = append(args, "-L", socket)
			}
			// send-keys with each key as its own argv element. This is the
			// ONE place CREW types into a pane, and it only ever sends the
			// literal key names below — never operator-supplied text, which
			// would be the injection surface spawn/ exists to avoid.
			args = append(args, "send-keys", "-t", paneID)
			args = append(args, keys...)
			return exec.CommandContext(ctx, "tmux", args...).Run()
		},
	}
}

// Clear polls the pane and answers whichever gate is currently showing,
// repeating until every required gate has been answered.
//
// Two design choices, both learned the hard way:
//
// It does NOT wait for gates in a fixed sequence. Sequencing forces a grace
// window on every optional gate — so hiring into an already-trusted directory
// (the common case) would stall for the full grace on a prompt that will never
// fire. Polling for whichever gate is present handles absent gates at zero
// cost and is order-independent.
//
// It never sends keys blind. A gate is answered only once its text is actually
// on the pane. A fixed-delay send would, on a fast machine, type Enter into
// the agent's prompt rather than the dialog — submitting an empty turn to a
// brand-new session.
func (g *GateClearer) Clear(ctx context.Context, paneID string) error {
	answered := make(map[string]bool, len(g.Gates))
	deadline := time.Now().Add(g.Timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if g.allRequiredAnswered(answered) {
			return nil
		}

		text, err := g.Capture(ctx, paneID)
		if err != nil {
			// A capture failure is transient (pane not ready yet); keep
			// polling until the deadline rather than aborting the hire.
			time.Sleep(g.Poll)
			continue
		}

		matched := false
		for _, gate := range g.Gates {
			if answered[gate.Name] || !strings.Contains(text, gate.Match) {
				continue
			}
			if err := g.SendKeys(ctx, paneID, gate.Keys); err != nil {
				return fmt.Errorf("gate %q: send keys: %w", gate.Name, err)
			}
			answered[gate.Name] = true
			matched = true
			break // one gate per frame; re-capture to see what's next
		}
		if !matched {
			time.Sleep(g.Poll)
		}
	}

	if g.allRequiredAnswered(answered) {
		return nil
	}
	for _, gate := range g.Gates {
		if !gate.Optional && !answered[gate.Name] {
			return fmt.Errorf("gate %q never appeared within %s: the session is "+
				"stuck somewhere unrecognised; check the pane", gate.Name, g.Timeout)
		}
	}
	return nil
}

func (g *GateClearer) allRequiredAnswered(answered map[string]bool) bool {
	for _, gate := range g.Gates {
		if !gate.Optional && !answered[gate.Name] {
			return false
		}
	}
	return true
}
