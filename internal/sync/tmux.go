package sync

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrTmuxUnreachable means the tmux command itself failed -- server down,
// binary missing, socket gone. Callers MUST treat this distinctly from a
// legitimately empty pane set: collapsing the two would read a dead tmux
// server as "every pane vanished," reporting every managed node as an
// independent death instead of surfacing one substrate-level fault.
var ErrTmuxUnreachable = errors.New("tmux: unreachable (server down or command failed)")

// ListPanes returns every pane across every tmux session on the box.
//
// On failure it returns (nil, error wrapping ErrTmuxUnreachable) -- never a
// non-nil empty map -- so a caller cannot mistake "the poll failed" for
// "there are truly zero panes right now."
func ListPanes() (map[string]Pane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id} #{pane_dead} #{pane_pid}").Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTmuxUnreachable, err)
	}
	return parsePanes(string(out)), nil
}

// parsePanes parses the output of
// `tmux list-panes -a -F '#{pane_id} #{pane_dead} #{pane_pid}'`.
// Pure -- no I/O -- so it's fully testable without a real tmux server.
//
// A malformed line is skipped rather than treated as a parse failure: a
// partial/garbled line from one pane shouldn't cost the whole tick's data
// for every other pane.
func parsePanes(output string) map[string]Pane {
	panes := map[string]Pane{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		id, deadFlag, pid := fields[0], fields[1], fields[2]
		panes[id] = Pane{ID: id, PID: pid, Dead: deadFlag == "1"}
	}
	return panes
}
