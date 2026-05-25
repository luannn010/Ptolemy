package store

import (
	"context"
	"database/sql"
	"time"
)

type migration struct {
	version string
	sql     string
}

var migrations = []migration{
	{
		version: "v1",
		sql: `
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
CREATE INDEX IF NOT EXISTS idx_command_logs_session_id ON command_logs(session_id);

CREATE TABLE IF NOT EXISTS policy_decisions (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	intent_kind TEXT NOT NULL,
	program TEXT NOT NULL,
	target TEXT NOT NULL DEFAULT '',
	effect TEXT NOT NULL,
	channel TEXT NOT NULL,
	rule_id TEXT NOT NULL,
	reason TEXT NOT NULL,
	intent_hash TEXT NOT NULL,
	confirmed INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_session_created_at
	ON policy_decisions(session_id, created_at);
`,
	},
}

func RunMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		);`); err != nil {
		return err
	}

	for _, m := range migrations {
		var appliedVersion string
		err := db.QueryRowContext(
			ctx,
			`SELECT version FROM schema_migrations WHERE version = ?`,
			m.version,
		).Scan(&appliedVersion)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			m.version,
			time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}
