// Package sync keeps CREW's view of the org live: polling tmux for pane
// state, detecting death, and reconciling against the broker/store. The
// decision logic is kept pure and table-tested, same discipline as layout/ —
// the I/O (shelling out to tmux, ticking on a timer) lives at the edges.
package sync

// Pane is one tmux pane's identity snapshot for a single tick.
type Pane struct {
	ID   string // tmux pane_id, e.g. "%3"
	PID  string // pane_pid
	Dead bool   // tmux's own pane_dead flag -- only observable if remain-on-exit is on
}

// PaneDiff is the pure result of comparing two ticks' worth of tmux state.
type PaneDiff struct {
	Died []string // pane IDs gone since last tick -- the primary death signal
	New  []string // pane IDs that appeared since last tick
}

// DiffPanes is a pure function: given the previous and current pane sets,
// determine what changed. No I/O.
//
// remain-on-exit defaults off in tmux, so a pane whose process exits is
// destroyed immediately rather than lingering with Dead=true — the primary
// death signal under default tmux behavior is a pane_id present last tick and
// absent now, not a boolean flag read. nil maps are treated as empty so the
// very first tick (no prior snapshot) doesn't need special-casing by callers.
func DiffPanes(prev, cur map[string]Pane) PaneDiff {
	var d PaneDiff
	for id, prevPane := range prev {
		curPane, ok := cur[id]
		// Same pane_id but a different pane_pid means the OS reused the id
		// for an unrelated pane (or tmux otherwise recycled it) -- treat the
		// old occupant as dead and the new one as freshly arrived, not as an
		// unchanged pane just because the map key matches.
		if !ok || curPane.PID != prevPane.PID {
			d.Died = append(d.Died, id)
		}
	}
	for id, curPane := range cur {
		prevPane, ok := prev[id]
		if !ok || prevPane.PID != curPane.PID {
			d.New = append(d.New, id)
		}
	}
	return d
}
