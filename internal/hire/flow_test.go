package hire

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/spawn"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// fakeAdapter records what it was asked to launch and returns a canned handle.
type fakeAdapter struct {
	handle spawn.Handle
	err    error
	specs  []spawn.Spec
}

func (f *fakeAdapter) Launch(ctx context.Context, s spawn.Spec) (spawn.Handle, error) {
	f.specs = append(f.specs, s)
	return f.handle, f.err
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/h.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testFlow(t *testing.T, a *fakeAdapter, peers func() ([]broker.Peer, error)) (*Flow, *store.Store) {
	t.Helper()
	st := testStore(t)
	n := 0
	// Flow refuses to spawn without consent (fails closed), so the fixture
	// grants it. The consent-refusal cases override this deliberately.
	consent := t.TempDir() + "/consent"
	if err := RecordConsent(consent); err != nil {
		t.Fatal(err)
	}
	return &Flow{
		ConsentPath: consent,
		Store:       st,
		Adapter:     a,
		ListPeers:   peers,
		IDFunc:      func() string { n++; return "node-" + string(rune('a'+n-1)) },
		BindTimeout: 300 * time.Millisecond,
		BindPoll:    5 * time.Millisecond,
	}, st
}

func req(t *testing.T) Request {
	return Request{Name: "backend-dev", Role: "dev", Workdir: t.TempDir(),
		Prompt: "You are a backend engineer."}
}

func nodeByID(t *testing.T, st *store.Store, id string) store.Node {
	t.Helper()
	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.NodeID == id {
			return n
		}
	}
	t.Fatalf("node %s not found", id)
	return store.Node{}
}

// The happy path: spawn, bind, alive.
func TestRunBindsPeerByTTY(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%7", TTY: "/dev/ttys042"}}
	f, st := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{
			{ID: "other", TTY: "ttys001"},
			{ID: "ours", TTY: "ttys042"}, // bare form, as the broker stores it
		}, nil
	})

	res, err := f.Run(context.Background(), req(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PeerID != "ours" {
		t.Fatalf("bound %q, want \"ours\" — tty matching picked the wrong peer", res.PeerID)
	}
	n := nodeByID(t, st, res.NodeID)
	if n.State != "alive" {
		t.Fatalf("state = %q, want alive", n.State)
	}
	if n.SpawnRef != "%7" {
		t.Fatalf("spawn_ref = %q, want %%7 (pane id)", n.SpawnRef)
	}
}

// THE BUG THAT WOULD HAVE BROKEN EVERY HIRE. tmux says /dev/ttys042, the
// broker stores ttys042. Without normalization nothing ever matches — and it
// fails SILENTLY, as a timeout that looks exactly like a slow session.
func TestRunNormalizesTTYAcrossTheFormatBoundary(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys099"}}
	f, st := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "p", TTY: "ttys099"}}, nil
	})
	res, err := f.Run(context.Background(), req(t))
	if err != nil {
		t.Fatalf("Run: %v — tmux's /dev/-prefixed tty did not match the broker's bare form", err)
	}
	if got := nodeByID(t, st, res.NodeID).BindTTY; got != "ttys099" {
		t.Fatalf("bind_tty stored as %q, want the normalized bare form", got)
	}
}

// A session that never registers must FAIL, not hang pending forever. A
// pending node that will never bind is indistinguishable from one still
// starting — 'failed' says the hire is over and where to look.
func TestRunMarksNodeFailedWhenBindTimesOut(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys777"}}
	f, st := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "someone-else", TTY: "ttys001"}}, nil
	})

	res, err := f.Run(context.Background(), req(t))
	if err == nil {
		t.Fatal("bind never happened but Run returned nil")
	}
	if !strings.Contains(err.Error(), "never registered") {
		t.Fatalf("error does not explain the failure: %v", err)
	}
	if n := nodeByID(t, st, res.NodeID); n.State != "failed" {
		t.Fatalf("state = %q, want failed — a hire that can never complete must "+
			"not sit in pending forever", n.State)
	}
}

// A launch failure must leave a VISIBLE failed node, not vanish. The pending
// row is written first precisely so this is possible.
func TestRunLeavesVisibleFailedNodeWhenLaunchFails(t *testing.T) {
	a := &fakeAdapter{err: errors.New("tmux exploded")}
	f, st := testFlow(t, a, func() ([]broker.Peer, error) { return nil, nil })

	res, err := f.Run(context.Background(), req(t))
	if err == nil {
		t.Fatal("launch failed but Run returned nil")
	}
	if res.NodeID == "" {
		t.Fatal("no NodeID returned — the caller cannot show the operator what failed")
	}
	if n := nodeByID(t, st, res.NodeID); n.State != "failed" {
		t.Fatalf("state = %q, want failed", n.State)
	}
}

// Operator free text must never reach a command line. It goes in a file, and
// spawn/ reads that file — the whole injection defense depends on this.
func TestRunPassesPromptViaFileNeverInline(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys1"}}
	f, _ := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "p", TTY: "ttys1"}}, nil
	})

	r := req(t)
	r.Brief = "'; touch /tmp/pwned; ' `whoami` $(id)"
	if _, err := f.Run(context.Background(), r); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(a.specs) != 1 {
		t.Fatalf("adapter launched %d times, want 1", len(a.specs))
	}
	spec := a.specs[0]
	if spec.PromptFile == "" {
		t.Fatal("no PromptFile — the brief must go via a file")
	}
	// The hostile brief must appear nowhere in Spec's scalar fields.
	for name, v := range map[string]string{
		"Name": spec.Name, "Role": spec.Role, "Workdir": spec.Workdir, "Mode": spec.Mode,
	} {
		if strings.Contains(v, "touch") {
			t.Fatalf("Spec.%s carries the hostile brief inline: %q", name, v)
		}
	}
}

// The prompt file must be cleaned up. It holds the role prompt and brief, and
// leaving temp files per hire in /tmp is a slow leak.
func TestRunCleansUpThePromptFile(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys1"}}
	f, _ := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "p", TTY: "ttys1"}}, nil
	})
	if _, err := f.Run(context.Background(), req(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.specs[0].PromptFile); err == nil {
		t.Fatal("prompt file still exists after Run — every hire leaks a temp file")
	}
}

// Validation happens before ANY side effect: no node row, no spawn.
func TestRunValidatesBeforeAnySideEffect(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys1"}}
	f, st := testFlow(t, a, func() ([]broker.Peer, error) { return nil, nil })

	for _, tc := range []struct {
		name string
		r    Request
	}{
		{"no name", Request{Workdir: t.TempDir()}},
		{"no workdir", Request{Name: "x"}},
		{"workdir missing", Request{Name: "x", Workdir: "/definitely/not/here"}},
	} {
		if _, err := f.Run(context.Background(), tc.r); err == nil {
			t.Fatalf("%s: want error", tc.name)
		}
	}
	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("%d node rows written for invalid requests — validation must "+
			"precede side effects", len(nodes))
	}
	if len(a.specs) != 0 {
		t.Fatalf("adapter launched %d times for invalid requests", len(a.specs))
	}
}
