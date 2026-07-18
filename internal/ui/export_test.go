package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'e' writes a JSON + Markdown snapshot into the launch dir and flashes the path.
func TestExportWritesSnapshotFiles(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	dir := t.TempDir()
	m.live.hireWorkdir = dir

	nm, cmd := m.Update(key("e"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("e produced no export command")
	}
	msg := cmd()
	res, ok := msg.(actionResultMsg)
	if !ok || !strings.Contains(res.text, "exported") {
		t.Fatalf("unexpected export result: %+v", msg)
	}

	// Exactly one .json and one .md snapshot should now exist in dir.
	entries, _ := os.ReadDir(dir)
	var json, md int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			json++
		}
		if strings.HasSuffix(e.Name(), ".md") {
			md++
		}
	}
	if json != 1 || md != 1 {
		t.Fatalf("expected 1 json + 1 md, got %d json, %d md in %s", json, md, filepath.Base(dir))
	}
}
