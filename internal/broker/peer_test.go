package broker

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func realBrokerPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path := filepath.Join(home, ".claude-peers.db")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no live broker db at %s, skipping: %v", path, err)
	}
	return path
}

// Against a real broker, not a fixture: the substrate's quirks (nullable
// columns, live process churn) are exactly what a mock would hide. This
// machine has several live peers from unrelated sessions at test time; we
// assert shape, never an exact count, since peers come and go.
func TestListPeersAgainstRealBroker(t *testing.T) {
	path := realBrokerPath(t)

	peers, err := ListPeers(path)
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) == 0 {
		t.Fatal("want at least one live peer on this machine, got zero")
	}
	for _, p := range peers {
		if p.ID == "" {
			t.Fatalf("peer %+v has empty ID", p)
		}
		if p.PID == 0 {
			t.Fatalf("peer %+v has zero PID", p)
		}
		normalized := NormalizeTTY(p.TTY)
		if p.TTY != "" && normalized == "" {
			t.Fatalf("peer %+v: normalizing TTY %q produced empty string", p, p.TTY)
		}
	}
}

// ListPeers must never let "broker unreachable" and "broker legitimately
// empty" collapse into the same return shape — a caller (sync/'s reconcile
// loop) has to be able to tell "the substrate is down" from "no agents
// exist" apart, the same distinction death-detection needs on the tmux side.
func TestListPeersOnMissingDBReturnsErrorNotEmptySlice(t *testing.T) {
	peers, err := ListPeers(filepath.Join(t.TempDir(), "does-not-exist.db"))
	if err == nil {
		t.Fatalf("want an error for a nonexistent broker db, got peers=%v, err=nil", peers)
	}
	if peers != nil {
		t.Fatalf("want a nil slice alongside the error, got %v", peers)
	}
}

// The mode=ro connection must reject writes, not merely happen to avoid
// making any. Verified against a copy of the real broker db so a would-be
// corrupting write is provably impossible at the connection layer, not just
// unattempted by our own code — the four unrelated MyHomebase peers on this
// machine depend on that db staying intact.
func TestBrokerConnectionRejectsWrites(t *testing.T) {
	src := realBrokerPath(t)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read real broker db: %v", err)
	}
	copyPath := filepath.Join(t.TempDir(), "copy.db")
	if err := os.WriteFile(copyPath, data, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	before, err := ListPeers(copyPath)
	if err != nil {
		t.Fatalf("ListPeers(copy): %v", err)
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", copyPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO peers (id, pid, cwd, tty, summary, registered_at, last_seen)
		VALUES ('injected', 1, '/tmp', 'ttys999', '', 'now', 'now')`)
	if err == nil {
		t.Fatal("write through a mode=ro connection succeeded — the read-only guarantee is not real")
	}

	after, err := ListPeers(copyPath)
	if err != nil {
		t.Fatalf("ListPeers(copy) after rejected write: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("peer count changed after a supposedly-rejected write: before=%d after=%d", len(before), len(after))
	}
}
