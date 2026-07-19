package store

import (
	"database/sql"
	"time"
)

// Strategy is a supervisor's restart policy for its direct children,
// mirroring Erlang/OTP's three canonical strategies. Lives in store (not
// package supervision) so both store and supervision can reference it
// without an import cycle: supervision.Evaluate takes []store.Node as input
// (matching lifecycle.Reparent/broker.Reconcile's existing directionality),
// so the policy/history types it also needs have to live on this side too.
type Strategy string

const (
	OneForOne  Strategy = "one-for-one"  // only the dead child restarts
	OneForAll  Strategy = "one-for-all"  // the dead child's death restarts every sibling
	RestForOne Strategy = "rest-for-one" // the dead child + siblings created at-or-after it
)

// Policy is one supervisor node's restart configuration.
type Policy struct {
	NodeID        string
	Strategy      Strategy
	MaxRestarts   int
	WindowSeconds int
}

// RestartEvent is one row of restart history, used to evaluate whether a new
// restart would exceed a supervisor's budget.
type RestartEvent struct {
	NodeID string
	At     time.Time
}

// InsertRestartEvent records one restart attempt against a node's history —
// the audit trail supervision.Evaluate's budget check reads back via
// ListRestartEvents. No FK to nodes (see schema.sql): the record must
// outlive a later fire/disband hard-delete.
func (s *Store) InsertRestartEvent(nodeID string, at time.Time, reason string) error {
	_, err := s.db.Exec(
		`INSERT INTO restarts (node_id, at, reason) VALUES (?,?,?)`,
		nodeID, at.UTC().Format(time.RFC3339Nano), reason)
	return err
}

// ListRestartEvents returns every recorded restart, oldest first.
// supervision.Evaluate filters by its own budget window; this returns the
// full history rather than pre-filtering, so the pure core stays the one
// place window logic lives.
func (s *Store) ListRestartEvents() ([]RestartEvent, error) {
	rows, err := s.db.Query(`SELECT node_id, at FROM restarts ORDER BY at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RestartEvent
	for rows.Next() {
		var nodeID, at string
		if err := rows.Scan(&nodeID, &at); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		out = append(out, RestartEvent{NodeID: nodeID, At: t})
	}
	return out, rows.Err()
}

// UpsertSupervisionPolicy inserts or edits a supervisor's restart policy.
// node_id is the primary key, so re-running with the same id is an edit —
// same idempotent-write style as UpsertRole.
func (s *Store) UpsertSupervisionPolicy(p Policy) error {
	_, err := s.db.Exec(
		`INSERT INTO supervision_policy (node_id, strategy, max_restarts, window_seconds)
		 VALUES (?,?,?,?)
		 ON CONFLICT(node_id) DO UPDATE SET
		   strategy = excluded.strategy,
		   max_restarts = excluded.max_restarts,
		   window_seconds = excluded.window_seconds`,
		p.NodeID, string(p.Strategy), p.MaxRestarts, p.WindowSeconds)
	return err
}

// GetSupervisionPolicy returns a node's explicit policy. ok is false (nil
// error) when none is set — supervision.Evaluate treats that as "use
// DefaultPolicy", a normal outcome, not a failure.
func (s *Store) GetSupervisionPolicy(nodeID string) (Policy, bool, error) {
	var p Policy
	var strategy string
	err := s.db.QueryRow(
		`SELECT node_id, strategy, max_restarts, window_seconds
		 FROM supervision_policy WHERE node_id = ?`, nodeID).
		Scan(&p.NodeID, &strategy, &p.MaxRestarts, &p.WindowSeconds)
	if err == sql.ErrNoRows {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, err
	}
	p.Strategy = Strategy(strategy)
	return p, true, nil
}

// ListSupervisionPolicies returns every explicit policy — what
// supervision.Evaluate needs alongside ListRestartEvents to make a decision;
// any node absent from this list falls back to supervision.DefaultPolicy.
func (s *Store) ListSupervisionPolicies() ([]Policy, error) {
	rows, err := s.db.Query(`SELECT node_id, strategy, max_restarts, window_seconds FROM supervision_policy`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Policy
	for rows.Next() {
		var p Policy
		var strategy string
		if err := rows.Scan(&p.NodeID, &strategy, &p.MaxRestarts, &p.WindowSeconds); err != nil {
			return nil, err
		}
		p.Strategy = Strategy(strategy)
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteSupervisionPolicy removes a node's explicit policy, reverting it to
// supervision.DefaultPolicy.
func (s *Store) DeleteSupervisionPolicy(nodeID string) error {
	_, err := s.db.Exec(`DELETE FROM supervision_policy WHERE node_id = ?`, nodeID)
	return err
}
