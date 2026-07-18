package resume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranscriptPathSlugifiesWorkdir(t *testing.T) {
	got := TranscriptPath("/home/me", "/Users/x/Desktop/App.test", "abc-123")
	want := filepath.Join("/home/me", ".claude", "projects", "-Users-x-Desktop-App-test", "abc-123.jsonl")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExistsFindsTranscriptAtSlugPath(t *testing.T) {
	home := t.TempDir()
	workdir := "/Users/x/proj"
	sid := "sess-1"
	dir := filepath.Dir(TranscriptPath(home, workdir, sid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(TranscriptPath(home, workdir, sid), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Exists(home, workdir, sid) {
		t.Fatal("Exists should find the transcript at the slug path")
	}
}

func TestExistsFallsBackToGlob(t *testing.T) {
	home := t.TempDir()
	sid := "sess-2"
	// Put the transcript under a DIFFERENT project dir than the workdir slug,
	// so only the glob fallback finds it.
	other := filepath.Join(home, ".claude", "projects", "some-other-slug")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, sid+".jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Exists(home, "/some/unrelated/workdir", sid) {
		t.Fatal("Exists should find the transcript via the glob fallback")
	}
}

func TestExistsFalseForMissingOrEmpty(t *testing.T) {
	home := t.TempDir()
	if Exists(home, "/x", "nope") {
		t.Fatal("missing session must not be reported as resumable")
	}
	if Exists(home, "/x", "") {
		t.Fatal("empty session id is never resumable")
	}
}
