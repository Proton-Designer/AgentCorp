package msg

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestBrokerDB builds a FRESH database from the verified schema
// (sqlite3 ~/.claude-peers.db ".schema messages", checked directly, not
// assumed) rather than copying the live broker's data. The live db holds
// real conversations belonging to other agents; there is no reason a test of
// the INSERT path needs any real message content, so it never touches it.
func newTestBrokerDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "broker.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open temp broker db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE peers (
			id TEXT PRIMARY KEY, pid INTEGER NOT NULL, cwd TEXT NOT NULL,
			git_root TEXT, tty TEXT, summary TEXT NOT NULL DEFAULT '',
			registered_at TEXT NOT NULL, last_seen TEXT NOT NULL
		);
		CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_id TEXT NOT NULL, to_id TEXT NOT NULL,
			text TEXT NOT NULL, sent_at TEXT NOT NULL,
			delivered INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (from_id) REFERENCES peers(id),
			FOREIGN KEY (to_id) REFERENCES peers(id)
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	_, err = db.Exec(`INSERT INTO peers (id, pid, cwd, registered_at, last_seen) VALUES
		('peer-a', 1, '/tmp', 't', 't'), ('peer-b', 2, '/tmp', 't', 't')`)
	if err != nil {
		t.Fatalf("seed peers: %v", err)
	}
	return path
}

func TestSendInsertsAnUndeliveredMessage(t *testing.T) {
	dbPath := newTestBrokerDB(t)

	if err := Send(dbPath, "peer-a", "peer-b", "take /bookings, gate on owner_id"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var fromID, toID, text, sentAt string
	var delivered int
	row := db.QueryRow(`SELECT from_id, to_id, text, sent_at, delivered FROM messages`)
	if err := row.Scan(&fromID, &toID, &text, &sentAt, &delivered); err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if fromID != "peer-a" || toID != "peer-b" || text != "take /bookings, gate on owner_id" {
		t.Fatalf("row = (%q,%q,%q), want (peer-a,peer-b,take /bookings, gate on owner_id)", fromID, toID, text)
	}
	if delivered != 0 {
		t.Fatalf("delivered = %d, want 0 (undelivered) -- the target's own poll marks it, not us", delivered)
	}
	if sentAt == "" {
		t.Fatal("sent_at was left empty")
	}
}

func TestSendMultipleMessagesDoNotOverwrite(t *testing.T) {
	dbPath := newTestBrokerDB(t)
	if err := Send(dbPath, "peer-a", "peer-b", "first"); err != nil {
		t.Fatal(err)
	}
	if err := Send(dbPath, "peer-a", "peer-b", "second"); err != nil {
		t.Fatal(err)
	}

	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM messages`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestSendOnMissingDBReturnsError(t *testing.T) {
	if err := Send(filepath.Join(t.TempDir(), "does-not-exist", "broker.db"), "a", "b", "x"); err == nil {
		t.Fatal("expected an error opening a db in a nonexistent directory")
	}
}
