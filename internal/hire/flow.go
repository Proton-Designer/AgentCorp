package hire

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/resume"
	"github.com/Proton-Designer/AgentCorp/internal/spawn"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// errBindPending marks a hire whose session was spawned and its gates cleared,
// but which had not registered with the broker by the bind deadline. It is NOT
// a failure: the node keeps its recorded bind_tty and stays pending, so the
// sync tick's PendingBind binds it automatically once the session registers.
// This is what makes a slow cold start self-heal instead of dying.
var errBindPending = errors.New("bind pending: session has not registered yet")

// Request is what the operator asked for.
type Request struct {
	Name     string
	Role     string
	Workdir  string
	ParentID string
	Brief    string // optional first task
	Prompt   string // the role's system prompt

	// RoleTemplate, if set, names a stored role whose prompt and glyph the flow
	// resolves — overriding Role and Prompt. Lets a hire pick "researcher"
	// instead of re-typing a system prompt every time.
	RoleTemplate string
}

// resolveRole applies a role template to a request: sets Role to the template
// name and composes Prompt as the agent's identity line plus the role's
// behavior description. Returns fellBack=true when a template was named but no
// longer exists (e.g. deleted between the picker and the hire) — the caller's
// Prompt is kept (never silently blanked), but the outcome is surfaced rather
// than passed off as a normal hire.
func (f *Flow) resolveRole(req *Request) (fellBack bool, err error) {
	if req.RoleTemplate == "" {
		return false, nil
	}
	r, ok, err := f.Store.GetRole(req.RoleTemplate)
	if err != nil {
		return false, fmt.Errorf("resolve role %q: %w", req.RoleTemplate, err)
	}
	if !ok {
		return true, nil // unknown template — keep the caller's Prompt, but say so
	}
	req.Role = r.Name
	req.Prompt = "You are a AgentCorp agent named " + req.Name + ".\n\n" + r.Prompt
	return false, nil
}

// Result reports what happened. NodeID always identifies the row so the caller
// can show the node rather than losing it. Pending is set when the session was
// spawned but had not registered by the bind deadline: not a failure — the node
// stays pending and the sync tick will bind it when the session appears.
type Result struct {
	NodeID  string
	PeerID  string
	Pending bool

	// RoleMissing names a role template that was requested but not found, so the
	// hire fell back to the default prompt. Empty on a normal hire. The UI
	// surfaces it instead of reporting a plain success for an agent that silently
	// lacks the role behavior the operator picked.
	RoleMissing string
}

// peerLister is injectable so the flow is testable without a live broker.
type peerLister func() ([]broker.Peer, error)

// Flow runs a hire end to end.
type Flow struct {
	Store     *store.Store
	Adapter   spawn.Adapter
	Gates     *GateClearer
	ListPeers peerLister
	IDFunc    func() string

	// ConsentPath is where the operator's first-run grant is recorded.
	//
	// Checked before every spawn, not once at startup: a long-running console
	// could otherwise outlive a revoked grant. Empty means unset, and unset
	// FAILS CLOSED — a bug that forgets to wire this must refuse to spawn
	// rather than silently spawn without consent.
	ConsentPath string

	// BindTimeout bounds the wait for the spawned session to register.
	// A session that never registers is FAILED, not pending forever —
	// a hire that hangs silently is the worst outcome, because it looks
	// identical to a slow one.
	BindTimeout time.Duration
	BindPoll    time.Duration
}

// Run creates the node, spawns the session, clears the gates, and binds.
//
// Ordering is not incidental (spec §6.1): the pending row is written FIRST so a
// crash anywhere after it leaves a visible failed node rather than an orphaned
// tmux pane nobody knows about. The pane's tty is captured BEFORE the session
// boots, because it is the binding key and there is no other way to know which
// of many registering peers is ours.
func (f *Flow) Run(ctx context.Context, req Request) (Result, error) {
	// Consent first — before validation, before any side effect. AgentCorp clears
	// a security prompt on the operator's behalf; doing that without their
	// informed, once-given agreement is the one thing this flow must never do.
	// Fails closed on an unset path: a wiring bug must refuse to spawn, not
	// spawn unconsented.
	if err := RequireConsent(f.ConsentPath); err != nil {
		return Result{}, err
	}
	// Resolve a role template (if any) before validation, so Role/Prompt reflect
	// the chosen archetype for the rest of the flow.
	fellBack, err := f.resolveRole(&req)
	if err != nil {
		return Result{}, err
	}
	if err := validate(req); err != nil {
		return Result{}, err
	}

	nodeID := f.IDFunc()
	res := Result{NodeID: nodeID}
	if fellBack {
		res.RoleMissing = req.RoleTemplate
	}

	// An AgentCorp-chosen session id, passed to `claude --session-id` and stored
	// on the node. This is what makes the agent revivable: a dead node whose
	// transcript still exists can be brought back with `claude --resume <id>`.
	sessionID := uuid.NewString()

	// 1. Prompt to a file. Never onto a command line — that's the injection
	//    surface spawn/ exists to avoid, and briefs are operator free text.
	promptFile, cleanup, err := writePrompt(req)
	if err != nil {
		return res, fmt.Errorf("write prompt: %w", err)
	}
	defer cleanup()

	// 2. Pending row first, so a failure after this point is VISIBLE.
	node := store.Node{
		NodeID:    nodeID,
		Name:      req.Name,
		Role:      req.Role,
		ParentID:  req.ParentID,
		Workdir:   req.Workdir,
		SpawnMode: "tmux-window",
		State:     "pending",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		SessionID: sessionID,
	}
	if err := f.Store.InsertNode(node); err != nil {
		return res, fmt.Errorf("insert pending node: %w", err)
	}

	// 3. Spawn. Handle.TTY is tmux's form (/dev/ttysNNN); the broker stores
	//    the bare form — normalize or nothing ever binds, silently.
	h, err := f.Adapter.Launch(ctx, spawn.Spec{
		Name:       req.Name,
		Role:       req.Role,
		Workdir:    req.Workdir,
		PromptFile: promptFile,
		Mode:       "tmux-window",
		SessionID:  sessionID,
	})
	if err != nil {
		f.fail(nodeID)
		return res, fmt.Errorf("launch: %w", err)
	}
	if err := f.Store.SetSpawnRef(nodeID, h.SpawnRef, broker.NormalizeTTY(h.TTY)); err != nil {
		f.fail(nodeID)
		return res, fmt.Errorf("record spawn ref: %w", err)
	}

	// 4. Clear the startup gates. The session will not reach the broker until
	//    both are answered.
	if f.Gates != nil {
		if err := f.Gates.Clear(ctx, h.SpawnRef); err != nil {
			f.fail(nodeID)
			return res, fmt.Errorf("clear gates: %w", err)
		}
	}

	// 5. Wait for the peer to appear, matched by tty.
	peerID, err := f.waitForBind(ctx, broker.NormalizeTTY(h.TTY))
	if err != nil {
		if errors.Is(err, errBindPending) {
			// Spawned and gates cleared, but the session hasn't registered by
			// the deadline — a slow cold start, not a failure. Leave the node
			// pending with its bind_tty intact; the sync tick's PendingBind will
			// bind it the moment it registers on that pane's tty.
			//
			// The tty-reuse risk spec §6.1 named is bounded by the pane's life:
			// the pane persists (remain-on-exit on) so its tty isn't reused while
			// the node waits, and if the pane dies, Decide's pane-death signal
			// tombstones the pending node (it has a spawn_ref) before any reuse
			// can matter. So "recoverable pending" can't silently mis-bind.
			res.Pending = true
			return res, nil
		}
		f.fail(nodeID)
		return res, err
	}
	if err := f.Store.BindPeer(nodeID, peerID); err != nil {
		f.fail(nodeID)
		return res, fmt.Errorf("bind: %w", err)
	}

	res.PeerID = peerID
	return res, nil
}

// ErrSessionGone means a dead agent can't be revived because its Claude Code
// transcript no longer exists on disk — there is no memory to resume. The
// caller surfaces this and offers to delete the node or adopt a replacement.
var ErrSessionGone = errors.New("session transcript not found — the agent's memory is gone")

// Revive brings a dead agent back with its memory: it respawns the agent's
// exact Claude session via `claude --resume <session-id>`, resets the node from
// dead to pending, and rebinds it exactly like a fresh hire. Refuses (with
// ErrSessionGone) if the transcript is missing, and refuses to revive a node
// that isn't dead or has no recorded session (adopted / pre-session-id).
//
// Spawn happens BEFORE the dead->pending reset, so a launch failure leaves the
// node dead (unchanged) rather than stranding it pending.
func (f *Flow) Revive(ctx context.Context, node store.Node) (Result, error) {
	if err := RequireConsent(f.ConsentPath); err != nil {
		return Result{}, err
	}
	res := Result{NodeID: node.NodeID}
	if node.State != "dead" {
		return res, fmt.Errorf("revive: %q is not dead", node.Name)
	}
	if node.SessionID == "" {
		return res, fmt.Errorf("revive: %q has no recorded session to resume", node.Name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return res, fmt.Errorf("revive: %w", err)
	}
	if !resume.Exists(home, node.Workdir, node.SessionID) {
		return res, ErrSessionGone
	}

	h, err := f.Adapter.Launch(ctx, spawn.Spec{
		Name:      node.Name,
		Role:      node.Role,
		Workdir:   node.Workdir,
		Mode:      "tmux-window",
		SessionID: node.SessionID,
		Resume:    true, // restore, don't start fresh; no prompt re-append
	})
	if err != nil {
		return res, fmt.Errorf("revive launch: %w", err)
	}
	if err := f.Store.Revive(node.NodeID); err != nil {
		return res, fmt.Errorf("revive reset: %w", err)
	}
	if err := f.Store.SetSpawnRef(node.NodeID, h.SpawnRef, broker.NormalizeTTY(h.TTY)); err != nil {
		return res, fmt.Errorf("revive record spawn ref: %w", err)
	}
	if f.Gates != nil {
		if err := f.Gates.Clear(ctx, h.SpawnRef); err != nil {
			return res, fmt.Errorf("revive clear gates: %w", err)
		}
	}
	peerID, err := f.waitForBind(ctx, broker.NormalizeTTY(h.TTY))
	if err != nil {
		if errors.Is(err, errBindPending) {
			res.Pending = true
			return res, nil
		}
		return res, err
	}
	if err := f.Store.BindPeer(node.NodeID, peerID); err != nil {
		return res, fmt.Errorf("revive bind: %w", err)
	}
	res.PeerID = peerID
	return res, nil
}

// waitForBind polls for a live peer whose tty matches the pane we launched into.
func (f *Flow) waitForBind(ctx context.Context, wantTTY string) (string, error) {
	if wantTTY == "" {
		return "", fmt.Errorf("no tty captured for the pane: cannot bind " +
			"(this is the binding key; without it the hire can never complete)")
	}
	deadline := time.Now().Add(f.BindTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		peers, err := f.ListPeers()
		if err == nil {
			for _, p := range peers {
				if broker.NormalizeTTY(p.TTY) == wantTTY {
					return p.ID, nil
				}
			}
		}
		time.Sleep(f.BindPoll)
	}
	// Deadline reached without a match. Wrap errBindPending so the caller can
	// distinguish "slow, keep the node pending for the tick to recover" from a
	// hard failure — while still carrying a descriptive message for logs.
	return "", fmt.Errorf("session did not register within %s (tty %s); "+
		"leaving it pending for background bind: %w", f.BindTimeout, wantTTY, errBindPending)
}

// fail marks a node failed rather than leaving it pending forever. A pending
// node that will never bind is indistinguishable from one still starting;
// 'failed' says the hire is over and shows the operator where to look.
func (f *Flow) fail(nodeID string) {
	_ = f.Store.SetState(nodeID, "failed")
}

func validate(req Request) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Workdir == "" {
		return fmt.Errorf("workdir is required")
	}
	fi, err := os.Stat(req.Workdir)
	if err != nil {
		return fmt.Errorf("workdir %q: %w", req.Workdir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("workdir %q is not a directory", req.Workdir)
	}
	return nil
}

// writePrompt puts the role prompt and brief in a file.
//
// Operator free text never touches a command line — spawn/ reads this file and
// passes its content as one argv element.
func writePrompt(req Request) (string, func(), error) {
	dir, err := os.MkdirTemp("", "agentcorp-prompt-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }

	body := req.Prompt
	if req.Brief != "" {
		body += "\n\nYour first task: " + req.Brief
	}
	path := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}
