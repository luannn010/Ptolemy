package store

import (
	"context"
	"database/sql"
)

func RunMigrations(ctx context.Context, db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS actions (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			type TEXT NOT NULL,
			input TEXT,
			output TEXT,
			status TEXT NOT NULL,
			metadata TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT
		);`,

		`CREATE TABLE IF NOT EXISTS logs (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			action_id TEXT,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			metadata TEXT,
			created_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS approvals (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			action_type TEXT NOT NULL,
			payload TEXT,
			status TEXT NOT NULL,
			reason TEXT,
			created_at TEXT NOT NULL,
			decided_at TEXT
		);`,

		`CREATE INDEX IF NOT EXISTS idx_actions_session_id ON actions(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_actions_status ON actions(status);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_session_id ON logs(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_action_id ON logs(action_id);`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_session_id ON approvals(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);`,
	}

	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	return nil
}
