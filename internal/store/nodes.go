package store

import (
	"database/sql"
	"fmt"
)

type Node struct {
	NodeID    string
	PeerID    string // "" = NULL (unbound)
	BindTTY   string
	Name      string
	Role      string
	ParentID  string // "" = NULL (root)
	Workdir   string
	SpawnMode string
	SpawnRef  string
	State     string // pending | alive | dead | failed
	CreatedAt string
	DiedAt    string // "" = NULL (not dead)
}

// nullify maps Go's empty string to SQL NULL. This matters for peer_id:
// SQLite exempts NULL from UNIQUE but treats "" as a real value, so storing ""
// would let exactly one node be unbound.
func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func str(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func (s *Store) InsertNode(n Node) error {
	_, err := s.db.Exec(`
		INSERT INTO nodes (node_id, peer_id, bind_tty, name, role, parent_id,
		                   workdir, spawn_mode, spawn_ref, state, created_at, died_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.NodeID, nullify(n.PeerID), nullify(n.BindTTY), n.Name, n.Role,
		nullify(n.ParentID), n.Workdir, n.SpawnMode, nullify(n.SpawnRef),
		n.State, n.CreatedAt, nullify(n.DiedAt))
	return err
}

// BindPeer binds a registered peer to a pending node.
//
// The state guard is load-bearing, not defensive clutter: without it, calling
// this on a tombstoned node silently resurrects it — and by then reparenting
// (§6.3) may have moved its children elsewhere, leaving a zombie parent and a
// chart that lies. pending -> alive is the only legal transition into alive.
func (s *Store) BindPeer(nodeID, peerID string) error {
	res, err := s.db.Exec(
		`UPDATE nodes SET peer_id = ?, state = 'alive'
		 WHERE node_id = ? AND state = 'pending'`,
		peerID, nodeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Loud, not silent: either the node doesn't exist or it wasn't pending.
		// A silent no-op here would surface later as "the agent spawned but
		// never appeared in the tree", which is the worst class of bug in this
		// project — it looks like a race and you debug the poll loop for a day.
		return fmt.Errorf("bind %s -> %s: node not found or not pending", nodeID, peerID)
	}
	return nil
}

func (s *Store) Tombstone(nodeID, when string) error {
	_, err := s.db.Exec(
		`UPDATE nodes SET state = 'dead', died_at = ? WHERE node_id = ?`,
		when, nodeID)
	return err
}

func (s *Store) ListNodes() ([]Node, error) {
	rows, err := s.db.Query(`
		SELECT node_id, peer_id, bind_tty, name, role, parent_id,
		       workdir, spawn_mode, spawn_ref, state, created_at, died_at
		FROM nodes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		var peerID, bindTTY, parentID, spawnRef, diedAt sql.NullString
		if err := rows.Scan(&n.NodeID, &peerID, &bindTTY, &n.Name, &n.Role,
			&parentID, &n.Workdir, &n.SpawnMode, &spawnRef,
			&n.State, &n.CreatedAt, &diedAt); err != nil {
			return nil, err
		}
		n.PeerID, n.BindTTY = str(peerID), str(bindTTY)
		n.ParentID, n.SpawnRef, n.DiedAt = str(parentID), str(spawnRef), str(diedAt)
		out = append(out, n)
	}
	return out, rows.Err()
}
