package apitypes

import (
	"time"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/memory"
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

// ChatRequest is the body of POST /chat on the RAG listener: a question for the
// memory RAG plus optional scope/trace controls.
type ChatRequest struct {
	Query     string `json:"query"`
	K         int    `json:"k,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Trace     bool   `json:"trace,omitempty"`
}

// ChatResponse carries the RAG answer. Mode and Steps appear only when the
// request set trace=true; gave_up=true is a successful, honest "not in the KB"
// (HTTP 200), mirroring memory.Answer semantics.
type ChatResponse struct {
	Answer    string             `json:"answer"`
	Citations []string           `json:"citations"`
	GaveUp    bool               `json:"gave_up"`
	Mode      string             `json:"mode,omitempty"`
	Steps     []memory.TraceStep `json:"steps,omitempty"`
}
