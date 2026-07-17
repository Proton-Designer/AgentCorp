package broker

import "strings"

// NormalizeTTY strips a "/dev/" prefix so a pane tty and the broker's stored
// tty can be compared for identity.
//
// This is the binding key for hire (spec §6.1) and load-bearing, not
// cosmetic: verified against live data on this machine, tmux reports
// "/dev/ttys024" (`tmux list-panes -a -F '#{pane_tty}'`) while the broker
// stores the bare "ttys000" (`sqlite3 ~/.claude-peers.db "SELECT tty FROM
// peers"`). Exact string equality between the two never matches, and would
// fail silently — every hire would sit in pending until timeout with nothing
// to explain why.
func NormalizeTTY(s string) string {
	return strings.TrimPrefix(s, "/dev/")
}
