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

	// SessionID is the Claude Code session id AgentCorp assigned when it spawned
	// this agent (via `claude --session-id`). It's what lets a dead agent be
	// revived with its memory intact (`claude --resume <id>`). Empty for adopted
	// agents (we didn't spawn them) and for pre-session-id nodes.
	SessionID string
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
		                   workdir, spawn_mode, spawn_ref, state, created_at, died_at,
		                   session_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.NodeID, nullify(n.PeerID), nullify(n.BindTTY), n.Name, n.Role,
		nullify(n.ParentID), n.Workdir, n.SpawnMode, nullify(n.SpawnRef),
		n.State, n.CreatedAt, nullify(n.DiedAt), nullify(n.SessionID))
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

// SetName renames a node. Names are how the UI layer identifies nodes, so the
// caller is responsible for rejecting a name that collides with another live
// node before calling this.
func (s *Store) SetName(nodeID, name string) error {
	_, err := s.db.Exec(`UPDATE nodes SET name = ? WHERE node_id = ?`, name, nodeID)
	return err
}

// DeleteNode removes a node row outright.
//
// Distinct from Tombstone on purpose. Tombstone is for a death AgentCorp merely
// *observed* (a crash the sync layer detected): the row survives so the operator
// still sees the node died and its children keep a valid parent. DeleteNode is
// for a death the operator *ordered* — fire or disband — where the node's
// children have already been reparented away, so nothing is orphaned and a
// lingering dead marker for a deliberate action is just clutter. The caller is
// responsible for having moved any children first.
func (s *Store) DeleteNode(nodeID string) error {
	_, err := s.db.Exec(`DELETE FROM nodes WHERE node_id = ?`, nodeID)
	return err
}

func (s *Store) ListNodes() ([]Node, error) {
	rows, err := s.db.Query(`
		SELECT node_id, peer_id, bind_tty, name, role, parent_id,
		       workdir, spawn_mode, spawn_ref, state, created_at, died_at, session_id
		FROM nodes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		var peerID, bindTTY, parentID, spawnRef, diedAt, sessionID sql.NullString
		if err := rows.Scan(&n.NodeID, &peerID, &bindTTY, &n.Name, &n.Role,
			&parentID, &n.Workdir, &n.SpawnMode, &spawnRef,
			&n.State, &n.CreatedAt, &diedAt, &sessionID); err != nil {
			return nil, err
		}
		n.PeerID, n.BindTTY = str(peerID), str(bindTTY)
		n.ParentID, n.SpawnRef, n.DiedAt = str(parentID), str(spawnRef), str(diedAt)
		n.SessionID = str(sessionID)
		out = append(out, n)
	}
	return out, rows.Err()
}

// Revive resets a dead node back to pending so it can be rebound to a freshly
// respawned session (the resume flow). It clears peer_id, bind_tty, spawn_ref,
// and died_at, and only acts on a currently-dead node — reviving anything else
// is a no-op guarded by the WHERE clause. The caller respawns the session
// (claude --resume) and records the new spawn_ref/bind_tty via SetSpawnRef,
// after which the sync tick binds it exactly like a fresh hire.
func (s *Store) Revive(nodeID string) error {
	res, err := s.db.Exec(
		`UPDATE nodes SET state = 'pending', peer_id = NULL, bind_tty = NULL,
		                  spawn_ref = NULL, died_at = NULL
		 WHERE node_id = ? AND state = 'dead'`, nodeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("revive %s: node not found or not dead", nodeID)
	}
	return nil
}

// SetSpawnRef records the tmux pane id and the normalized bind tty.
//
// Called after Launch and before the bind poll: bind_tty is the key the poll
// matches on (spec §6.1), and spawn_ref is the pane id the death-detection
// pane-diff compares against (§9).
func (s *Store) SetSpawnRef(nodeID, spawnRef, bindTTY string) error {
	res, err := s.db.Exec(
		`UPDATE nodes SET spawn_ref = ?, bind_tty = ? WHERE node_id = ?`,
		nullify(spawnRef), nullify(bindTTY), nodeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("set spawn ref: node %s not found", nodeID)
	}
	return nil
}

// SetState moves a node to a terminal non-alive state (failed).
//
// Deliberately refuses 'alive' and 'dead': alive has exactly one legal entry
// point (BindPeer, guarded to pending-only) and dead has one (Tombstone, which
// also stamps died_at). A general-purpose state setter would let a caller
// bypass either guard — which is how a tombstoned node gets resurrected.
func (s *Store) SetState(nodeID, state string) error {
	if state != "failed" && state != "pending" {
		return fmt.Errorf("SetState refuses %q: use BindPeer for alive, "+
			"Tombstone for dead — those transitions are guarded", state)
	}
	res, err := s.db.Exec(`UPDATE nodes SET state = ? WHERE node_id = ?`, state, nodeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("set state: node %s not found", nodeID)
	}
	return nil
}

// SetParent re-attaches a node to a new parent (reparenting after a fire).
// Empty newParent makes the node a root.
func (s *Store) SetParent(nodeID, newParent string) error {
	res, err := s.db.Exec(`UPDATE nodes SET parent_id = ? WHERE node_id = ?`,
		nullify(newParent), nodeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("set parent: node %s not found", nodeID)
	}
	return nil
}
