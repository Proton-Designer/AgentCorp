// Package broker is the read side of the substrate: how CREW learns what
// agents actually exist. ~/.claude-peers.db is owned by claude-peers, not us
// — we never write, migrate, or lock it (spec §9). Connections are opened
// mode=ro so that property holds even if a bug tries to violate it: verified
// empirically against a copy of the live broker db, an INSERT through a
// mode=ro connection fails with "attempt to write a readonly database (8)"
// rather than silently succeeding.
package broker

import (
	"database/sql"
	"fmt"
)

// Peer is a row from the broker's peers table, verified against the live
// schema (sqlite3 ~/.claude-peers.db ".schema peers") rather than assumed
// from the reference doc.
type Peer struct {
	ID           string
	PID          int
	CWD          string
	GitRoot      string // "" if not inside a git repo (column is nullable)
	TTY          string // "" if undeterminable (column is nullable)
	Summary      string
	RegisteredAt string
	LastSeen     string
}

// ListPeers reads the broker's live peer list.
//
// A legitimately empty broker and a broken read must never collapse into the
// same shape: any I/O failure (db unreachable, corrupt, locked) returns a
// non-nil error with a nil slice; a broker that is simply reachable and empty
// returns a non-nil empty slice with a nil error. Callers must not treat "err
// == nil && len(peers) == 0" as "broker down" — that conflation is exactly
// what would make a stale/missing broker file look identical to a quiet
// company, the same failure mode sync/ must avoid for tmux list-panes.
func ListPeers(dbPath string) ([]Peer, error) {
	peers := []Peer{}
	err := queryReadOnly(dbPath, `
		SELECT id, pid, cwd, git_root, tty, summary, registered_at, last_seen
		FROM peers`,
		func(rows *sql.Rows) error {
			var p Peer
			var gitRoot, tty sql.NullString
			if err := rows.Scan(&p.ID, &p.PID, &p.CWD, &gitRoot, &tty,
				&p.Summary, &p.RegisteredAt, &p.LastSeen); err != nil {
				return fmt.Errorf("scan peer: %w", err)
			}
			if gitRoot.Valid {
				p.GitRoot = gitRoot.String
			}
			if tty.Valid {
				p.TTY = tty.String
			}
			peers = append(peers, p)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return peers, nil
}
