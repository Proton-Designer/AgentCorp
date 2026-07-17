package broker

import (
	"os/exec"
)

// KillPane terminates a tmux pane by id (%N). Used to retire a spawned agent.
//
// This kills the PANE, which SIGHUPs the claude process in it — that triggers
// the session's own /unregister on a clean exit, and the broker's PID reap
// catches it either way. We do NOT touch the broker DB to remove the peer;
// that's the substrate's job, and writing to a database we don't own is
// exactly what the read-only discipline forbids everywhere else.
//
// A pane that's already gone is success: the agent exited on its own.
func KillPane(paneID string) error {
	if paneID == "" {
		return nil
	}
	// -t targets the pane; kill-pane on a missing pane errors, which we ignore
	// at the caller since "already dead" is the desired end state.
	return exec.Command("tmux", "kill-pane", "-t", paneID).Run()
}
