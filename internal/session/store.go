package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/luannn010/ptolemy/internal/store"
)

var ErrSessionNotFound = errors.New("session not found")

type Store struct {
	store *store.Store
}

func NewStore(s *store.Store) *Store {
	return &Store{store: s}
}

func (s *Store) Create(ctx context.Context, req CreateSessionRequest) (Session, error) {
	now := time.Now().UTC()

	sess := Session{
		ID:          uuid.NewString(),
		Name:        req.Name,
		Status:      StatusOpen,
		Workspace:   req.Workspace,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	query := `
	INSERT INTO sessions (
		id, name, status, workspace, description, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?);
	`

	_, err := s.store.DB.ExecContext(
		ctx,
		query,
		sess.ID,
		sess.Name,
		string(sess.Status),
		sess.Workspace,
		sess.Description,
		sess.CreatedAt.Format(time.RFC3339),
		sess.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}

	return sess, nil
}

func (s *Store) Get(ctx context.Context, id string) (Session, error) {
	query := `
	SELECT id, name, status, workspace, description, created_at, updated_at, closed_at
	FROM sessions
	WHERE id = ?;
	`

	row := s.store.DB.QueryRowContext(ctx, query, id)

	sess, err := scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}

	return sess, nil
}

func (s *Store) List(ctx context.Context) ([]Session, error) {
	query := `
	SELECT id, name, status, workspace, description, created_at, updated_at, closed_at
	FROM sessions
	ORDER BY created_at DESC;
	`

	rows, err := s.store.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session

	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}

	return sessions, rows.Err()
}

func (s *Store) CloseSession(ctx context.Context, id string) (Session, error) {
	now := time.Now().UTC()

	query := `
	UPDATE sessions
	SET status = ?, updated_at = ?, closed_at = ?
	WHERE id = ?;
	`

	res, err := s.store.DB.ExecContext(
		ctx,
		query,
		string(StatusClosed),
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
		id,
	)
	if err != nil {
		return Session{}, fmt.Errorf("close session: %w", err)
	}

	count, err := res.RowsAffected()
	if err != nil {
		return Session{}, err
	}

	if count == 0 {
		return Session{}, ErrSessionNotFound
	}

	return s.Get(ctx, id)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (Session, error) {
	var sess Session
	var status string
	var createdAt string
	var updatedAt string
	var closedAt sql.NullString

	err := row.Scan(
		&sess.ID,
		&sess.Name,
		&status,
		&sess.Workspace,
		&sess.Description,
		&createdAt,
		&updatedAt,
		&closedAt,
	)
	if err != nil {
		return Session{}, err
	}

	sess.Status = Status(status)

	parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Session{}, err
	}

	parsedUpdatedAt, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return Session{}, err
	}

	sess.CreatedAt = parsedCreatedAt
	sess.UpdatedAt = parsedUpdatedAt

	if closedAt.Valid {
		parsedClosedAt, err := time.Parse(time.RFC3339, closedAt.String)
		if err != nil {
			return Session{}, err
		}
		sess.ClosedAt = &parsedClosedAt
	}

	return sess, nil
}
