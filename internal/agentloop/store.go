package agentloop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrRunNotFound = errors.New("agent run not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateRun(ctx context.Context, run Run) (Run, error) {
	now := time.Now().UTC()
	run.ID = uuid.NewString()
	run.CreatedAt = now
	run.UpdatedAt = now
	if run.Status == "" {
		run.Status = StatusPending
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO agent_runs (
			id, session_id, task_id, task_file, workspace, branch, worktree_path, status,
			finalization_mode, current_step, max_steps, invalid_json_count, same_action_retry_count,
			no_progress_count, current_phase, last_error, final_report_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		nullIfEmpty(run.SessionID),
		run.TaskID,
		run.TaskFile,
		run.Workspace,
		run.Branch,
		run.WorktreePath,
		run.Status,
		run.FinalizationMode,
		run.CurrentStep,
		run.MaxSteps,
		run.InvalidJSONCount,
		run.SameActionRetryCount,
		run.NoProgressCount,
		run.CurrentPhase,
		run.LastError,
		run.FinalReportPath,
		run.CreatedAt.Format(time.RFC3339),
		run.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Run{}, err
	}

	return run, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		id, session_id, task_id, task_file, workspace, branch, worktree_path, finalization_mode, status,
		current_step, max_steps, invalid_json_count, same_action_retry_count,
		no_progress_count, current_phase, last_error, final_report_path, created_at, updated_at
		FROM agent_runs
		WHERE id = ?`, id)

	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, ErrRunNotFound
		}
		return Run{}, err
	}

	return run, nil
}

func (s *Store) UpdateRun(ctx context.Context, run Run) (Run, error) {
	now := time.Now().UTC()
	run.UpdatedAt = now

	res, err := s.db.ExecContext(
		ctx,
		`UPDATE agent_runs
		SET session_id = ?, task_id = ?, task_file = ?, workspace = ?, branch = ?, worktree_path = ?,
		    finalization_mode = ?,
		    status = ?, current_step = ?, max_steps = ?, invalid_json_count = ?,
		    same_action_retry_count = ?, no_progress_count = ?, current_phase = ?,
		    last_error = ?, final_report_path = ?, updated_at = ?
		WHERE id = ?`,
		nullIfEmpty(run.SessionID),
		run.TaskID,
		run.TaskFile,
		run.Workspace,
		run.Branch,
		run.WorktreePath,
		run.FinalizationMode,
		run.Status,
		run.CurrentStep,
		run.MaxSteps,
		run.InvalidJSONCount,
		run.SameActionRetryCount,
		run.NoProgressCount,
		run.CurrentPhase,
		run.LastError,
		run.FinalReportPath,
		run.UpdatedAt.Format(time.RFC3339),
		run.ID,
	)
	if err != nil {
		return Run{}, err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return Run{}, err
	}
	if rowsAffected == 0 {
		return Run{}, ErrRunNotFound
	}

	return s.GetRun(ctx, run.ID)
}

func (s *Store) ListObservations(ctx context.Context, runID string) ([]Observation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, run_id, action_id, step, source, summary, artifact_path, raw_payload, created_at
		FROM agent_observations
		WHERE run_id = ?
		ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []Observation
	for rows.Next() {
		observation, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}

	return observations, rows.Err()
}

func (s *Store) CreateObservation(ctx context.Context, observation Observation) (Observation, error) {
	now := time.Now().UTC()
	observation.ID = uuid.NewString()
	observation.CreatedAt = now

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO agent_observations
		(id, run_id, action_id, step, source, summary, artifact_path, raw_payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID,
		observation.RunID,
		nullIfEmpty(observation.ActionID),
		observation.Step,
		observation.Source,
		observation.Summary,
		observation.ArtifactPath,
		observation.RawPayload,
		observation.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Observation{}, err
	}

	return observation, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var sessionID sql.NullString
	var createdAt string
	var updatedAt string

	err := row.Scan(
		&run.ID,
		&sessionID,
		&run.TaskID,
		&run.TaskFile,
		&run.Workspace,
		&run.Branch,
		&run.WorktreePath,
		&run.FinalizationMode,
		&run.Status,
		&run.CurrentStep,
		&run.MaxSteps,
		&run.InvalidJSONCount,
		&run.SameActionRetryCount,
		&run.NoProgressCount,
		&run.CurrentPhase,
		&run.LastError,
		&run.FinalReportPath,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return Run{}, err
	}
	if sessionID.Valid {
		run.SessionID = sessionID.String
	}

	run.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Run{}, fmt.Errorf("parse created_at: %w", err)
	}
	run.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return Run{}, fmt.Errorf("parse updated_at: %w", err)
	}

	return run, nil
}

func scanObservation(row scanner) (Observation, error) {
	var observation Observation
	var actionID sql.NullString
	var createdAt string

	err := row.Scan(
		&observation.ID,
		&observation.RunID,
		&actionID,
		&observation.Step,
		&observation.Source,
		&observation.Summary,
		&observation.ArtifactPath,
		&observation.RawPayload,
		&createdAt,
	)
	if err != nil {
		return Observation{}, err
	}
	if actionID.Valid {
		observation.ActionID = actionID.String
	}
	observation.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Observation{}, fmt.Errorf("parse created_at: %w", err)
	}

	return observation, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
