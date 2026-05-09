package action

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
}

type Action struct {
	ID            string
	AgentRunID    string
	SessionID     string
	Type          string
	Input         string
	Output        string
	Status        string
	Metadata      string
	Title         string
	Purpose       string
	ReasoningStep string
	Target        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, a Action) (Action, error) {
	now := time.Now().UTC()

	a.ID = uuid.NewString()
	a.CreatedAt = now
	a.UpdatedAt = now
	a.Metadata = MergeMetadata(a.Metadata, ActionMetadata{
		Title:         a.Title,
		Purpose:       a.Purpose,
		ReasoningStep: a.ReasoningStep,
		Target:        a.Target,
	})

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO actions 
		(id, agent_run_id, session_id, type, input, output, status, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID,
		nullIfEmpty(a.AgentRunID),
		a.SessionID,
		a.Type,
		a.Input,
		a.Output,
		a.Status,
		a.Metadata,
		a.CreatedAt.Format(time.RFC3339),
		a.UpdatedAt.Format(time.RFC3339),
	)

	return a, err
}

func (s *Store) ListBySession(ctx context.Context, sessionID string) ([]Action, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, agent_run_id, session_id, type, input, output, status, metadata, created_at, updated_at
		FROM actions
		WHERE session_id = ?
		ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []Action
	for rows.Next() {
		var item Action
		var agentRunID sql.NullString
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&item.ID,
			&agentRunID,
			&item.SessionID,
			&item.Type,
			&item.Input,
			&item.Output,
			&item.Status,
			&item.Metadata,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		if agentRunID.Valid {
			item.AgentRunID = agentRunID.String
		}

		item.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		meta := ParseMetadata(item.Metadata)
		item.Title = meta.Title
		item.Purpose = meta.Purpose
		item.ReasoningStep = meta.ReasoningStep
		item.Target = meta.Target

		actions = append(actions, item)
	}

	return actions, rows.Err()
}

func (s *Store) ListByRun(ctx context.Context, runID string) ([]Action, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, agent_run_id, session_id, type, input, output, status, metadata, created_at, updated_at
		FROM actions
		WHERE agent_run_id = ?
		ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []Action
	for rows.Next() {
		var item Action
		var agentRunID sql.NullString
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&item.ID,
			&agentRunID,
			&item.SessionID,
			&item.Type,
			&item.Input,
			&item.Output,
			&item.Status,
			&item.Metadata,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		if agentRunID.Valid {
			item.AgentRunID = agentRunID.String
		}

		item.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		meta := ParseMetadata(item.Metadata)
		item.Title = meta.Title
		item.Purpose = meta.Purpose
		item.ReasoningStep = meta.ReasoningStep
		item.Target = meta.Target

		actions = append(actions, item)
	}

	return actions, rows.Err()
}

func (s *Store) UpdateResult(ctx context.Context, id, output, status string) error {
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`UPDATE actions
		 SET output = ?, status = ?, updated_at = ?
		 WHERE id = ?`,
		output,
		status,
		now.Format(time.RFC3339),
		id,
	)

	return err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
