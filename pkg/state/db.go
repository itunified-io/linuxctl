// Package state owns the SQLite-backed run history used by apply / rollback.
package state

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB pointing at ~/.linuxctl/state.db.
type DB struct{ inner *sql.DB }

// Open opens or creates the state database.
func Open(path string) (*DB, error) {
	if path == "" {
		return &DB{}, nil
	}
	inner, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}
	return &DB{inner: inner}, nil
}

// Close closes the underlying database.
func (d *DB) Close() error {
	if d == nil || d.inner == nil {
		return nil
	}
	return d.inner.Close()
}
