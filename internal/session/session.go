package session

import "time"

type Status string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

type Session struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      Status    `json:"status"`
	Workspace   string    `json:"workspace"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

type CreateSessionRequest struct {
	Name        string `json:"name"`
	Workspace   string `json:"workspace"`
	Description string `json:"description"`
}