package store

import (
	"context"
	"database/sql"
	"strings"
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

		`CREATE TABLE IF NOT EXISTS agent_runs (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			task_id TEXT NOT NULL DEFAULT '',
			task_file TEXT NOT NULL DEFAULT '',
			workspace TEXT NOT NULL DEFAULT '',
			branch TEXT NOT NULL DEFAULT '',
			worktree_path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			current_step INTEGER NOT NULL DEFAULT 0,
			max_steps INTEGER NOT NULL DEFAULT 0,
			invalid_json_count INTEGER NOT NULL DEFAULT 0,
			same_action_retry_count INTEGER NOT NULL DEFAULT 0,
			no_progress_count INTEGER NOT NULL DEFAULT 0,
			current_phase TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			final_report_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS agent_observations (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			action_id TEXT,
			step INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			artifact_path TEXT NOT NULL DEFAULT '',
			raw_payload TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS program_runs (
			id TEXT PRIMARY KEY,
			program_id TEXT NOT NULL DEFAULT '',
			program_name TEXT NOT NULL DEFAULT '',
			mode TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			workspace TEXT NOT NULL DEFAULT '',
			current_pack_id TEXT NOT NULL DEFAULT '',
			current_task_id TEXT NOT NULL DEFAULT '',
			percent_complete REAL NOT NULL DEFAULT 0,
			total_packs INTEGER NOT NULL DEFAULT 0,
			completed_packs INTEGER NOT NULL DEFAULT 0,
			total_tasks INTEGER NOT NULL DEFAULT 0,
			completed_tasks INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS pack_runs (
			id TEXT PRIMARY KEY,
			program_run_id TEXT NOT NULL,
			pack_id TEXT NOT NULL,
			pack_name TEXT NOT NULL DEFAULT '',
			pack_path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			current_task_id TEXT NOT NULL DEFAULT '',
			percent_complete REAL NOT NULL DEFAULT 0,
			total_tasks INTEGER NOT NULL DEFAULT 0,
			completed_tasks INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS pack_run_tasks (
			id TEXT PRIMARY KEY,
			pack_run_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			task_path TEXT NOT NULL DEFAULT '',
			branch TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			agent_run_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			execution_group TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			depends_on_json TEXT NOT NULL DEFAULT '[]',
			validation_json TEXT NOT NULL DEFAULT '[]',
			allowed_files_json TEXT NOT NULL DEFAULT '[]',
			checklist_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS run_events (
			id TEXT PRIMARY KEY,
			program_run_id TEXT NOT NULL,
			pack_run_id TEXT NOT NULL DEFAULT '',
			pack_run_task_id TEXT NOT NULL DEFAULT '',
			agent_run_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			data TEXT NOT NULL DEFAULT '',
			artifact_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	if err := addColumnIfMissing(ctx, db, `ALTER TABLE actions ADD COLUMN agent_run_id TEXT`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, db, `ALTER TABLE agent_runs ADD COLUMN finalization_mode TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}

	indexQueries := []string{
		`CREATE INDEX IF NOT EXISTS idx_actions_session_id ON actions(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_actions_agent_run_id ON actions(agent_run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_actions_status ON actions(status);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_session_id ON logs(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_action_id ON logs(action_id);`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_session_id ON approvals(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_session_id ON agent_runs(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_observations_run_id ON agent_observations(run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_program_runs_status ON program_runs(status);`,
		`CREATE INDEX IF NOT EXISTS idx_pack_runs_program_run_id ON pack_runs(program_run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pack_run_tasks_pack_run_id ON pack_run_tasks(pack_run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pack_run_tasks_agent_run_id ON pack_run_tasks(agent_run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_run_events_program_run_id ON run_events(program_run_id);`,
	}

	for _, q := range indexQueries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	return nil
}

func addColumnIfMissing(ctx context.Context, db *sql.DB, query string) error {
	if _, err := db.ExecContext(ctx, query); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return nil
		}
		return err
	}
	return nil
}
