package logs

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

func (s *Store) Create(ctx context.Context, l Log) (Log, error) {
	l.ID = uuid.NewString()
	l.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO logs
		(id, session_id, action_id, level, message, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID,
		l.SessionID,
		l.ActionID,
		l.Level,
		l.Message,
		l.Metadata,
		l.CreatedAt.Format(time.RFC3339),
	)

	return l, err
}
