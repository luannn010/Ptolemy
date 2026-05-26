package apitypes

import (
	"time"

	"github.com/luannn010/ptolemy/internal/domain"
)

type CreateSessionRequest struct {
	Name        string `json:"name"`
	Workspace   string `json:"workspace"`
	Description string `json:"description"`
}

type SessionResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Workspace   string     `json:"workspace"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

type SessionListResponse struct {
	Sessions []SessionResponse `json:"sessions"`
}

type RunCommandRequest struct {
	Command      string `json:"command"`
	CWD          string `json:"cwd"`
	Timeout      int    `json:"timeout"`
	ConfirmToken string `json:"confirm_token,omitempty"`
	PendingID    string `json:"pending_id,omitempty"`
}

type NeedsConfirmation struct {
	Status     string         `json:"status"`
	Channel    domain.Channel `json:"channel"`
	IntentHash string         `json:"intent_hash,omitempty"`
	PendingID  string         `json:"pending_id,omitempty"`
	Reason     string         `json:"reason"`
}

type ConfirmRequest struct {
	ConfirmToken string `json:"confirm_token,omitempty"`
	PendingID    string `json:"pending_id,omitempty"`
}

type ErrorResponse struct {
	Error  string `json:"error"`
	RuleID string `json:"rule_id,omitempty"`
}
