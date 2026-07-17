package broker

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// queryReadOnly opens dbPath mode=ro, runs query, calls scan once per row,
// and closes both the rows and the db before returning — every broker
// reader goes through here, so the connection lifecycle is handled in one
// place instead of once per table.
//
// This is also the one place the read-only guarantee is expressed: the
// mode=ro DSN. Verified empirically (not just assumed) that the driver
// actually enforces it: see TestBrokerConnectionRejectsWrites in
// peer_test.go, which copies the live broker db, opens it through this exact
// path, and confirms an INSERT fails with "attempt to write a readonly
// database".
func queryReadOnly(dbPath, query string, scan func(*sql.Rows) error) error {
	dsn := fmt.Sprintf("file:%s?mode=ro", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open broker db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
