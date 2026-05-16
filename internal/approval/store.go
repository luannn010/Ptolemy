package approval

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
}

type Approval struct {
	ID         string
	SessionID  string
	ActionType string
	Payload    string
	Status     string
	Reason     string
	CreatedAt  time.Time
	DecidedAt  *time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, a Approval) (Approval, error) {
	a.ID = uuid.NewString()
	a.CreatedAt = time.Now().UTC()

	if a.Status == "" {
		a.Status = "pending"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approvals
		(id, session_id, action_type, payload, status, reason, created_at, decided_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID,
		a.SessionID,
		a.ActionType,
		a.Payload,
		a.Status,
		a.Reason,
		a.CreatedAt.Format(time.RFC3339),
		nil,
	)

	return a, err
}

func (s *Store) Decide(ctx context.Context, id, status, reason string) error {
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`UPDATE approvals
		 SET status = ?, reason = ?, decided_at = ?
		 WHERE id = ?`,
		status,
		reason,
		now.Format(time.RFC3339),
		id,
	)

	return err
}
