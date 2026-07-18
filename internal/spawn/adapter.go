// Package spawn creates the real OS process half of a node (spec §4): a
// terminal running an interactive claude session for a hired agent.
//
// Every field on Spec may carry operator-supplied text — a hire modal's
// name, a brief, a working directory — and REQUIREMENTS LC-4 treats every
// one of them as hostile. The defense is structural, not a filter: every
// value is passed to exec.Command as its own argv element and never joined
// into a string that could be re-parsed by a shell or by tmux's own command
// chaining (`cmd1 \; cmd2`). See tmux.go's doc comment for the empirical
// verification behind that claim — this package does not assume tmux's
// argv-handling, it was checked against the real binary before this code
// was written.
package spawn

import "context"

// Spec describes what to launch. Name and Workdir may be operator-supplied
// and must never be interpolated into a shell string (see package doc).
type Spec struct {
	Name string // display name for the terminal (e.g. tmux window name)

	// Role is the node's identity label (spec §6.4) — glyph/metadata only.
	// It plays no part in command construction here: the role's *behavior*
	// (system prompt) is already baked into PromptFile by the caller before
	// Launch is invoked. Carried on Spec for adapters that may want it for
	// display (e.g. a native tab title), unused by TmuxWindowAdapter today.
	Role string

	Workdir string // must exist and be a directory; validated before launch

	// PromptFile is a path to a file whose CONTENT is appended to claude's
	// system prompt. The file's content — read by this package, then passed
	// as one argv element — is what reaches claude; the path itself never
	// appears on anything resembling a shell command line, and the content
	// is never typed into the pane as keystrokes.
	PromptFile string

	Mode string // "tmux-window" (v1) or "" (defaults to tmux-window)

	// SessionID is the Claude Code session id for this launch. On a fresh hire
	// it's an AgentCorp-generated id passed via `--session-id` so AgentCorp can
	// later resume it. On a revive (Resume=true) it's the stored id of the dead
	// agent's session, passed via `--resume` to bring it back with its memory.
	// It's a UUID AgentCorp controls, never operator text — but it still travels
	// as its own argv element like everything else.
	SessionID string

	// Resume, when true, launches `claude --resume <SessionID>` (restore an
	// existing session) rather than `claude --session-id <SessionID>` (start a
	// new one with a chosen id). The system prompt is not re-appended on resume:
	// the session already carries its identity and history.
	Resume bool
}

// Handle identifies a launched session for later tty-binding (spec §6.1)
// and death detection (spec §8).
type Handle struct {
	// SpawnRef is the tmux PANE id (%N) — never the window id (@N). Spec
	// §9's spawn_ref contract: pane_id -> window_id is derivable, the
	// reverse isn't (a window can hold N panes), and §8's death-detection
	// pane-set diff compares pane_ids, so a spawn_ref in the wrong
	// namespace would fail to match silently.
	SpawnRef string

	// TTY is the pane's tty as tmux reports it (e.g. "/dev/ttys024"), NOT
	// yet normalized. The caller normalizes via broker.NormalizeTTY before
	// storing or matching — spawn/ has no reason to import broker/ just to
	// call one string function on its own output.
	TTY string
}

// Adapter creates a terminal running a claude session for a hired node.
type Adapter interface {
	Launch(ctx context.Context, spec Spec) (Handle, error)
}
