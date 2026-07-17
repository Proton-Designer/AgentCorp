package store

import (
	"database/sql"
	"testing"
)

// SQLite ignores FK constraints by default. If this fails, parent_id is
// decorative and nothing else in the data model can be trusted (spec §9).
func TestOpenEnforcesForeignKeys(t *testing.T) {
	s, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var fk int
	if err := s.DB().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}

// The pragma must hold across pooled connections, not just the first one.
// This is the failure mode a one-off Exec would have.
func TestForeignKeysHoldAcrossPooledConnections(t *testing.T) {
	s, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Force several distinct connections to be live at once.
	s.DB().SetMaxOpenConns(4)
	var conns []*sql.Conn
	for i := 0; i < 4; i++ {
		c, err := s.DB().Conn(t.Context())
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		var fk int
		if err := c.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatalf("conn %d pragma: %v", i, err)
		}
		if fk != 1 {
			t.Fatalf("conn %d: foreign_keys = %d, want 1", i, fk)
		}
		c.Close()
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := t.TempDir() + "/test.db"
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open on existing db: %v", err)
	}
	s2.Close()
}
