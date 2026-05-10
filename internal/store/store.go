package store

import (
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

	if err := s.migrate(); err != nil {
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

func (s *Store) migrate() error {
	queries := []string{
		`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			workspace TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			closed_at TEXT
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS command_logs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			command TEXT NOT NULL,
			cwd TEXT NOT NULL,
			exit_code INTEGER NOT NULL,
			output TEXT NOT NULL,
			error_output TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		);
		`,
	}

	for _, query := range queries {
		if _, err := s.DB.Exec(query); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}
func (s *Store) SQLDB() *sql.DB {
	return s.DB
}
