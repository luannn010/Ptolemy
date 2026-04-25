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
	ID        string
	SessionID string
	Type      string
	Input     string
	Output    string
	Status    string
	Metadata  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, a Action) (Action, error) {
	now := time.Now().UTC()

	a.ID = uuid.NewString()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO actions 
		(id, session_id, type, input, output, status, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID,
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
