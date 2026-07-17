// Package msg is AgentCorp's honest messaging layer: the one place AgentCorp writes
// to claude-peers' broker, plus the pure logic for classifying where a
// message actually came from and for keeping "sent" and "acted on" — two
// very different claims — impossible to conflate.
package msg

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Send writes a new row to the broker's messages table — the one write path
// AgentCorp has; every other access to that database (internal/broker) is
// mode=ro. The target's own MCP server polls this row up and, if its
// session's channel is active, pushes it into the live model's context.
// AgentCorp has no way to observe whether or when that actually happens — see
// DeliveryState.
//
// Opened as a plain read-write connection, deliberately NOT layering on
// stricter guarantees than the live system itself applies (e.g. this
// package's own store/ enables foreign_keys=ON as a considered choice for
// data AgentCorp owns; this database belongs to a third party, and imposing our
// own convention on it risks rejecting inserts the real claude-peers server
// would have accepted).
func Send(dbPath, from, to, text string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("msg: open broker db: %w", err)
	}
	defer db.Close()

	sentAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = db.Exec(
		`INSERT INTO messages (from_id, to_id, text, sent_at, delivered) VALUES (?, ?, ?, ?, 0)`,
		from, to, text, sentAt)
	if err != nil {
		return fmt.Errorf("msg: insert: %w", err)
	}
	return nil
}
