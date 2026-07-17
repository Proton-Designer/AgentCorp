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

func TestInCompanyKeepsOnlyInCompany(t *testing.T) {
	root := t.TempDir()
	if _, err := company.Create(root, "Acme", "co-1"); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "svc")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir() // a different tree, no company

	peers := []broker.Peer{
		{ID: "p-in-root", CWD: root},
		{ID: "p-in-sub", CWD: inside},
		{ID: "p-out", CWD: outside},
	}
	got := InCompany(canonDir(t, root), peers)

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

func TestInCompanyUnscopedReturnsAll(t *testing.T) {
	peers := []broker.Peer{{ID: "a", CWD: "/tmp/x"}, {ID: "b", CWD: "/tmp/y"}}
	got := InCompany("", peers)
	if len(got) != 2 {
		t.Fatalf("unscoped should pass all peers through, got %d", len(got))
	}
}
