package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type CommandLog struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Command     string    `json:"command"`
	CWD         string    `json:"cwd"`
	ExitCode    int       `json:"exit_code"`
	Output      string    `json:"output"`
	ErrorOutput string    `json:"error_output"`
	DurationMS  int64     `json:"duration_ms"`
	CreatedAt   time.Time `json:"created_at"`
}

type RunCommandRequest struct {
	Command       string `json:"command"`
	CWD           string `json:"cwd"`
	Timeout       int    `json:"timeout"`
	Title         string `json:"title,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
	ReasoningStep string `json:"reasoning_step,omitempty"`
	Target        string `json:"target,omitempty"`
}

type Confirmation struct {
	PendingID string         `json:"pending_id"`
	Channel   domain.Channel `json:"channel"`
	Reason    string         `json:"reason"`
}

type RunResult struct {
	Log          *CommandLog   `json:"log,omitempty"`
	Confirmation *Confirmation `json:"confirmation,omitempty"`
}

type GuardedRunner interface {
	Run(ctx context.Context, sessionID string, command string, cwd string, timeoutSeconds int) (terminal.Result, error)
}

type Service struct {
	runner GuardedRunner
	logs   *Store
}

func NewService(runner GuardedRunner, logs *Store) *Service {
	return &Service{runner: runner, logs: logs}
}

func (s *Service) Run(ctx context.Context, sessionID string, req RunCommandRequest) (RunResult, error) {
	runResult, err := s.runner.Run(ctx, sessionID, req.Command, req.CWD, req.Timeout)
	if err != nil {
		var needs policy.ErrNeedsConfirmation
		if errors.As(err, &needs) {
			return RunResult{
				Confirmation: &Confirmation{
					PendingID: needs.PendingID,
					Channel:   needs.Channel,
					Reason:    needs.Reason,
				},
			}, nil
		}
		return RunResult{}, err
	}

	logItem, err := s.logs.Create(ctx, CommandLog{
		ID:          uuid.NewString(),
		SessionID:   sessionID,
		Command:     req.Command,
		CWD:         req.CWD,
		ExitCode:    runResult.ExitCode,
		Output:      runResult.Output,
		ErrorOutput: runResult.ErrorOutput,
		DurationMS:  runResult.DurationMS,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		return RunResult{}, err
	}

	return RunResult{Log: &logItem}, nil
}
