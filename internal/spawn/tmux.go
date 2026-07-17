package spawn

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TmuxWindowAdapter launches each node as its own tmux window (spec §8,
// SP-2/SP-3, the v1 default).
//
// Launch never builds a shell command string. Verified empirically against
// the real tmux binary (3.7b) before writing this, not assumed from the man
// page:
//   - `tmux respawn-pane -t <pane> -- <tok1> <tok2> ...`, given multiple
//     tokens as SEPARATE argv elements (exactly what exec.Command produces —
//     no shell in between), execs tok1 directly with the rest as its argv.
//     A payload like `'; touch /tmp/x; '`, backticks, or `$(...)` embedded in
//     one of those tokens is passed through literally; nothing is
//     re-parsed by /bin/sh.
//   - `tmux new-window -n <name>`, given a hostile name as one argv element
//     (e.g. `'; touch /tmp/x; '` or “ `touch /tmp/x` “), creates a window
//     with that literal string as its name. tmux's own command-chaining
//     syntax (`cmd1 \; cmd2`) requires `;` as its OWN separate argv token —
//     an embedded `;` inside a flag's value does not trigger it.
//   - A literal newline in a window name is rejected outright by tmux
//     ("invalid window name") — a clean error, not silent misbehavior.
//
// The one rule that keeps all of this true: every field that can carry
// operator text is passed to exec.Command as its own argv element, never
// joined with another value into a single string.
type TmuxWindowAdapter struct {
	// Socket selects the tmux server via `-L <socket>`. Empty uses tmux's
	// default server — the one the CREW console itself runs in (spec §8).
	Socket string

	// claudeArgs builds the argv for the process launched into the pane,
	// given the prompt file's content. Defaults to the real claude
	// invocation. Tests override it with a harmless command so Launch can
	// be exercised end-to-end — including every real tmux call — without
	// spawning a real claude process or touching the live broker.
	claudeArgs func(promptContent string) []string
}

// NewTmuxWindowAdapter constructs an adapter targeting the given tmux
// socket ("" for the default server).
func NewTmuxWindowAdapter(socket string) *TmuxWindowAdapter {
	return &TmuxWindowAdapter{Socket: socket, claudeArgs: defaultClaudeArgs}
}

func defaultClaudeArgs(promptContent string) []string {
	// Mirrors spec §6.1 step 4. The prompt content is one argv element —
	// never a "$(cat file)" shell substitution, which is what that section
	// forbids: naive string interpolation into anything shell-parsed.
	return []string{
		"claude",
		"--dangerously-load-development-channels", "server:claude-peers",
		"--append-system-prompt", promptContent,
	}
}

func (a *TmuxWindowAdapter) tmux(ctx context.Context, args ...string) *exec.Cmd {
	if a.Socket != "" {
		args = append([]string{"-L", a.Socket}, args...)
	}
	return exec.CommandContext(ctx, "tmux", args...)
}

// run executes a tmux command and returns its stdout, with stderr folded
// into the error on failure so a rejected call (e.g. "invalid window name")
// surfaces its real reason instead of just a bare exit status.
func run(cmd *exec.Cmd) ([]byte, error) {
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s: %s", cmd.Args, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("%s: %w", cmd.Args, err)
	}
	return out, nil
}

func (a *TmuxWindowAdapter) Launch(ctx context.Context, spec Spec) (Handle, error) {
	if spec.Mode != "" && spec.Mode != "tmux-window" {
		return Handle{}, fmt.Errorf("spawn: TmuxWindowAdapter does not implement mode %q", spec.Mode)
	}

	info, err := os.Stat(spec.Workdir)
	if err != nil {
		return Handle{}, fmt.Errorf("spawn: workdir: %w", err)
	}
	if !info.IsDir() {
		return Handle{}, fmt.Errorf("spawn: workdir %q is not a directory", spec.Workdir)
	}

	promptBytes, err := os.ReadFile(spec.PromptFile)
	if err != nil {
		return Handle{}, fmt.Errorf("spawn: read prompt file: %w", err)
	}

	// Capture the pane id now. tty is captured AFTER respawn-pane below — see
	// the comment there. -n and -c each carry their own value as a separate
	// argv element; tmux never re-parses either as a command.
	out, err := run(a.tmux(ctx, "new-window", "-d", "-P", "-F", "#{pane_id}",
		"-n", spec.Name, "-c", spec.Workdir))
	if err != nil {
		return Handle{}, fmt.Errorf("spawn: tmux new-window: %w", err)
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return Handle{}, fmt.Errorf("spawn: tmux new-window returned no pane id")
	}

	// Retain a crashed agent's final output for debugging (spec §8, SP-5) —
	// scoped to this one window's pane, never set globally.
	if _, err := run(a.tmux(ctx, "set-option", "-t", paneID, "remain-on-exit", "on")); err != nil {
		return Handle{}, fmt.Errorf("spawn: tmux set-option remain-on-exit: %w", err)
	}

	// Replace the pane's shell with the real command via argv (execve, no
	// shell reparse) rather than typing a string into the pane with
	// send-keys, which would hand the text to the pane's live shell to
	// interpret.
	args := append([]string{"respawn-pane", "-k", "-t", paneID, "--"}, a.claudeArgs(string(promptBytes))...)
	if _, err := run(a.tmux(ctx, args...)); err != nil {
		return Handle{}, fmt.Errorf("spawn: tmux respawn-pane: %w", err)
	}

	// Capture the tty AFTER respawn-pane, NOT before. Verified against real
	// tmux: respawn-pane -k kills the pane's process and starts a new one on a
	// FRESH pseudo-terminal, so the tty changes (e.g. ttys021 -> ttys022) while
	// the pane id stays fixed. The tty is the broker binding key; capturing it
	// from the pre-respawn shell records a tty that no live peer will ever
	// have, and every hire silently fails to bind. (This is exactly the bug a
	// real first hire surfaced — the adversarial tests used /bin/true and never
	// compared the tty to a registering session.)
	ttyOut, err := run(a.tmux(ctx, "display-message", "-p", "-t", paneID, "#{pane_tty}"))
	if err != nil {
		return Handle{}, fmt.Errorf("spawn: read pane tty after respawn: %w", err)
	}
	tty := strings.TrimSpace(string(ttyOut))

	return Handle{SpawnRef: paneID, TTY: tty}, nil
}
