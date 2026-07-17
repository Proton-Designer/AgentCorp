package hire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/spawn"
	"github.com/aymanmohammed/crew/internal/store"
)

// Request is what the operator asked for.
type Request struct {
	Name     string
	Role     string
	Workdir  string
	ParentID string
	Brief    string // optional first task
	Prompt   string // the role's system prompt
}

// Result reports what happened. Both fields are meaningful on failure: NodeID
// always identifies the row so the caller can show a failed node rather than
// losing it.
type Result struct {
	NodeID string
	PeerID string
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
	// Consent first — before validation, before any side effect. CREW clears
	// a security prompt on the operator's behalf; doing that without their
	// informed, once-given agreement is the one thing this flow must never do.
	// Fails closed on an unset path: a wiring bug must refuse to spawn, not
	// spawn unconsented.
	if err := RequireConsent(f.ConsentPath); err != nil {
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
	return "", fmt.Errorf("session never registered with the broker within %s "+
		"(tty %s): it may be stuck at a prompt, or the channel flag may not be set",
		f.BindTimeout, wantTTY)
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
	dir, err := os.MkdirTemp("", "crew-prompt-")
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
