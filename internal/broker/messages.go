package broker

import "database/sql"

// Message is a row from the broker's messages table, verified against the
// live schema (sqlite3 ~/.claude-peers.db ".schema messages") — including an
// FK to peers(id) on both from_id and to_id that isn't mentioned in the
// design doc, discovered by checking rather than assuming.
type Message struct {
	ID        int64
	FromID    string
	ToID      string
	Text      string
	SentAt    string
	Delivered bool
}

// ListMessages reads the broker's message history, oldest first.
//
// Same read-only discipline as ListPeers (mode=ro, verified to reject
// writes) and the same error-vs-empty distinction: any I/O failure returns a
// non-nil error with a nil slice; a broker with no messages yet returns a
// non-nil empty slice with a nil error.
//
// This table holds other agents' real conversations — on this machine, four
// unrelated MyHomebase peers are actively messaging through it right now.
// Callers may derive counts, buckets, and single most-recent excerpts from
// Text (that's what the ticker is for), but nothing in this package logs,
// echoes, or dumps message bodies — see messages_test.go for how the tests
// verify shape without printing content.
func ListMessages(dbPath string) ([]Message, error) {
	msgs := []Message{}
	err := queryReadOnly(dbPath, `
		SELECT id, from_id, to_id, text, sent_at, delivered
		FROM messages ORDER BY sent_at ASC`,
		func(rows *sql.Rows) error {
			var m Message
			var delivered int
			if err := rows.Scan(&m.ID, &m.FromID, &m.ToID, &m.Text, &m.SentAt, &delivered); err != nil {
				return err
			}
			m.Delivered = delivered != 0
			msgs = append(msgs, m)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return msgs, nil
}
