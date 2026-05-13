package command

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/luannn010/ptolemy/internal/store"
)

type Store struct {
	store *store.Store
}

func NewStore(s *store.Store) *Store {
	return &Store{store: s}
}

func (s *Store) Create(ctx context.Context, log CommandLog) (CommandLog, error) {
	if log.ID == "" {
		log.ID = uuid.NewString()
	}

	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO command_logs (
		id, session_id, command, cwd, exit_code, output, error_output, duration_ms, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := s.store.DB.ExecContext(
		ctx,
		query,
		log.ID,
		log.SessionID,
		log.Command,
		log.CWD,
		log.ExitCode,
		log.Output,
		log.ErrorOutput,
		log.DurationMS,
		log.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return CommandLog{}, fmt.Errorf("create command log: %w", err)
	}

	return log, nil
}

func (s *Store) ListBySession(ctx context.Context, sessionID string) ([]CommandLog, error) {
	query := `
	SELECT id, session_id, command, cwd, exit_code, output, error_output, duration_ms, created_at
	FROM command_logs
	WHERE session_id = ?
	ORDER BY created_at DESC;
	`

	rows, err := s.store.DB.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list command logs: %w", err)
	}
	defer rows.Close()

	var logs []CommandLog

	for rows.Next() {
		var item CommandLog
		var createdAt string

		if err := rows.Scan(
			&item.ID,
			&item.SessionID,
			&item.Command,
			&item.CWD,
			&item.ExitCode,
			&item.Output,
			&item.ErrorOutput,
			&item.DurationMS,
			&createdAt,
		); err != nil {
			return nil, err
		}

		parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}

		item.CreatedAt = parsedCreatedAt
		logs = append(logs, item)
	}

	return logs, rows.Err()
}
