package hire

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
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
// behavior description. A no-op when RoleTemplate is empty; an unknown template
// is left to the caller's Prompt (never silently blanks the system prompt).
func (f *Flow) resolveRole(req *Request) error {
	if req.RoleTemplate == "" {
		return nil
	}
	r, ok, err := f.Store.GetRole(req.RoleTemplate)
	if err != nil {
		return fmt.Errorf("resolve role %q: %w", req.RoleTemplate, err)
	}
	if !ok {
		return nil // unknown template — keep whatever Prompt the caller set
	}
	req.Role = r.Name
	req.Prompt = "You are a AgentCorp agent named " + req.Name + ".\n\n" + r.Prompt
	return nil
}

// Result reports what happened. NodeID always identifies the row so the caller
// can show the node rather than losing it. Pending is set when the session was
// spawned but had not registered by the bind deadline: not a failure — the node
// stays pending and the sync tick will bind it when the session appears.
type Result struct {
	NodeID  string
	PeerID  string
	Pending bool
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
	if err := f.resolveRole(&req); err != nil {
		return Result{}, err
	}
	if err := validate(req); err != nil {
		return Result{}, err
	}

	nodeID := f.IDFunc()
	res := Result{NodeID: nodeID}

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
