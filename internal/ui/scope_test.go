package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
)

func canonDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return resolved
}

func TestScopedPeersKeepsOnlyInCompany(t *testing.T) {
	root := t.TempDir()
	if _, err := company.Create(root, "Acme", "co-1"); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "svc")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir() // a different tree, no company

	raw := func() ([]broker.Peer, error) {
		return []broker.Peer{
			{ID: "p-in-root", CWD: root},
			{ID: "p-in-sub", CWD: inside},
			{ID: "p-out", CWD: outside},
		}, nil
	}

	got, err := ScopedPeers(canonDir(t, root), raw)()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, p := range got {
		ids[p.ID] = true
	}
	if !ids["p-in-root"] || !ids["p-in-sub"] {
		t.Fatalf("in-company peers dropped: %v", ids)
	}
	if ids["p-out"] {
		t.Fatalf("out-of-company peer leaked in: %v", ids)
	}
}

func TestScopedPeersUnscopedReturnsAll(t *testing.T) {
	raw := func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "a", CWD: "/tmp/x"}, {ID: "b", CWD: "/tmp/y"}}, nil
	}
	got, err := ScopedPeers("", raw)()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("unscoped should pass all peers through, got %d", len(got))
	}
}

func TestScopedPeersPropagatesReadError(t *testing.T) {
	sentinel := os.ErrPermission
	raw := func() ([]broker.Peer, error) { return nil, sentinel }
	_, err := ScopedPeers("/some/root", raw)()
	if err != sentinel {
		t.Fatalf("expected the raw read error to propagate, got %v", err)
	}
}
