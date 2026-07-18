// Package resume locates Claude Code session transcripts so AgentCorp can tell
// whether a dead agent is actually revivable — i.e. whether its memory still
// exists on disk to `claude --resume`.
package resume

import (
	"os"
	"path/filepath"
	"strings"
)

// TranscriptPath returns where Claude Code stores a session's transcript:
// <home>/.claude/projects/<slug(workdir)>/<sessionID>.jsonl. The slug is the
// working directory with '/' and '.' replaced by '-', which is Claude Code's
// own project-directory naming.
func TranscriptPath(home, workdir, sessionID string) string {
	return filepath.Join(home, ".claude", "projects", slugify(workdir), sessionID+".jsonl")
}

func slugify(p string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(p)
}

// Exists reports whether a resumable transcript for sessionID is present. It
// checks the expected slug path first, then falls back to a glob across every
// project directory — so a small difference in how the workdir was slugified
// never produces a false "the memory is gone". An empty sessionID is never
// resumable (adopted agents, or nodes from before session ids were recorded).
func Exists(home, workdir, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	if _, err := os.Stat(TranscriptPath(home, workdir, sessionID)); err == nil {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	return len(matches) > 0
}
