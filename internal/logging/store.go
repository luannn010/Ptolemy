package logging

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
}

type Log struct {
	ID        string
	SessionID string
	ActionID  string
	Level     string
	Message   string
	Metadata  string
	CreatedAt time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, entry Log) (Log, error) {
	now := time.Now().UTC()

	entry.ID = uuid.NewString()
	entry.CreatedAt = now

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO logs
		(id, session_id, action_id, level, message, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.SessionID,
		nullIfEmpty(entry.ActionID),
		entry.Level,
		entry.Message,
		entry.Metadata,
		entry.CreatedAt.Format(time.RFC3339),
	)

	return entry, err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
