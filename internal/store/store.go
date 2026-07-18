// Package store owns AgentCorp's sidecar database — the hierarchy, roles, and node
// metadata that claude-peers' broker knows nothing about.
//
// This package never writes to ~/.claude-peers.db. That database is owned by
// the substrate and is read-only to us (spec §9).
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct{ db *sql.DB }

// Open opens (creating if needed) the sidecar database and applies the schema.
//
// The foreign_keys pragma goes in the DSN rather than a one-off Exec on
// purpose: SQLite ignores FK constraints unless enabled, the setting is
// per-connection, and database/sql pools connections. A single
// `Exec("PRAGMA foreign_keys=ON")` would apply to whichever pooled connection
// happened to serve it and silently not the others — leaving parent_id
// decorative on an unpredictable subset of queries.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	// Migrate existing databases: CREATE TABLE IF NOT EXISTS is a no-op on an
	// existing table, so a column added to the schema after a DB was created
	// won't appear without this. ADD COLUMN is idempotent-by-intent here — a
	// "duplicate column name" error just means the migration already ran, which
	// is success, not failure.
	if _, err := db.Exec(`ALTER TABLE nodes ADD COLUMN session_id TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		return nil, fmt.Errorf("migrate session_id: %w", err)
	}
	s := &Store{db: db}
	// Seed default role archetypes into a fresh store (no-op if roles exist),
	// so a new company can hire a "researcher"/"engineer"/"reviewer" without
	// first defining one.
	if err := s.SeedDefaultRoles(); err != nil {
		return nil, fmt.Errorf("seed roles: %w", err)
	}
	return s, nil
}

func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Close() error { return s.db.Close() }
