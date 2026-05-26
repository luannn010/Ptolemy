package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	configureSQLite(db)
	if err := applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	s := &Store{DB: db}

	if err := RunMigrations(context.Background(), s.DB); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func configureSQLite(db *sql.DB) {
	// Keep SQLite on a single shared connection to avoid concurrent writer locks.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA journal_mode = WAL;",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("configure sqlite pragma %q: %w", pragma, err)
		}
	}

	return nil
}
func (s *Store) SQLDB() *sql.DB {
	return s.DB
}
