package sync

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// --- pure parser tests (no I/O) ---

func TestParsePanesEmptyOutput(t *testing.T) {
	got := parsePanes("")
	if len(got) != 0 {
		t.Fatalf("parsePanes(\"\") = %v, want empty map", got)
	}
}

func TestParsePanesSingleLine(t *testing.T) {
	got := parsePanes("%3 0 4242\n")
	want := map[string]Pane{"%3": {ID: "%3", PID: "4242", Dead: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePanes = %+v, want %+v", got, want)
	}
}

func TestParsePanesDeadFlag(t *testing.T) {
	got := parsePanes("%1 1 111\n")
	if !got["%1"].Dead {
		t.Fatalf("pane_dead=1 should set Dead=true, got %+v", got["%1"])
	}
}

func TestParsePanesMultipleLinesAndTrailingWhitespace(t *testing.T) {
	got := parsePanes("%1 0 111\n%2 0 222\n\n")
	if len(got) != 2 {
		t.Fatalf("got %d panes, want 2: %+v", len(got), got)
	}
	if got["%1"].PID != "111" || got["%2"].PID != "222" {
		t.Fatalf("unexpected parse: %+v", got)
	}
}

func TestParsePanesMalformedLineIsSkippedNotFatal(t *testing.T) {
	// A short/garbled line shouldn't panic or corrupt the rest of the parse --
	// it should be skipped, since a partial tmux read is better recovered from
	// by ignoring the bad line than by losing the whole tick's data.
	got := parsePanes("%1 0 111\ngarbage\n%2 0 222\n")
	if len(got) != 2 {
		t.Fatalf("got %d panes, want 2 (malformed line skipped): %+v", len(got), got)
	}
}

// --- I/O integration tests (real tmux, isolated, cleaned up) ---

func skipIfNoTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH, skipping integration test")
	}
}

func TestListPanesSeesARealIsolatedSession(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "crew-sync-test-listpanes"
	// Belt-and-suspenders: make sure no stale session from a prior failed run
	// is lying around before we start.
	exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})

	if out, err := exec.Command("tmux", "new-session", "-d", "-s", sessionName).CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v: %s", err, out)
	}

	paneOut, err := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("tmux list-panes (setup check): %v", err)
	}
	paneID := string(paneOut)
	for len(paneID) > 0 && (paneID[len(paneID)-1] == '\n' || paneID[len(paneID)-1] == '\r') {
		paneID = paneID[:len(paneID)-1]
	}

	panes, err := ListPanes()
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	if _, ok := panes[paneID]; !ok {
		t.Fatalf("ListPanes() did not include our isolated session's pane %q; got %+v", paneID, panes)
	}
}

func TestListPanesReturnsErrTmuxUnreachableOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH-swap fake binary approach targets unix shells")
	}

	// Put a fake `tmux` on PATH ahead of the real one that always fails, so we
	// exercise the real error-wrapping code path without touching the actual
	// shared tmux server other live peer sessions depend on.
	dir := t.TempDir()
	fake := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'no server running' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	_, err := ListPanes()
	if err == nil {
		t.Fatal("ListPanes() with a failing tmux binary returned nil error, want ErrTmuxUnreachable")
	}
	if !errors.Is(err, ErrTmuxUnreachable) {
		t.Fatalf("ListPanes() error = %v, want it to wrap ErrTmuxUnreachable", err)
	}
}

// A failed list-panes must never be silently read as "zero panes" by a
// caller diffing against a previous tick -- that would flatline every
// managed node in the UI simultaneously instead of surfacing one substrate
// fault. This pins the actual failure-mode contract: callers MUST check the
// error before touching the (nil) map, and DiffPanes must never be handed a
// nil map as a stand-in for "the poll failed" -- only for "truly zero panes."
func TestListPanesFailureReturnsNilMapNotEmptyMap(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "tmux")
	os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	panes, err := ListPanes()
	if err == nil {
		t.Fatal("expected an error")
	}
	if panes != nil {
		t.Fatalf("ListPanes() on failure returned a non-nil map %+v; callers must not be able to mistake this for a legitimately empty result", panes)
	}
}
