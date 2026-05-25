package executor

import (
	"context"
	"fmt"
	"strings"

	actionpkg "github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type Executor struct {
	sessionStore *session.Store
	commandStore *command.Store
	actionStore  *actionpkg.Store
	logStore     *logging.Store
	runner       *terminal.TmuxRunner
}

func NewExecutor(
	sessionStore *session.Store,
	commandStore *command.Store,
	actionStore *actionpkg.Store,
	logStore *logging.Store,
	runner *terminal.TmuxRunner,
) *Executor {
	return &Executor{
		sessionStore: sessionStore,
		commandStore: commandStore,
		actionStore:  actionStore,
		logStore:     logStore,
		runner:       runner,
	}
}

type ExecuteRequest struct {
	SessionID     string `json:"session_id"`
	Command       string `json:"command"`
	CWD           string `json:"cwd"`
	Reason        string `json:"reason"`
	Timeout       int    `json:"timeout"`
	Title         string `json:"title,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
	ReasoningStep string `json:"reasoning_step,omitempty"`
	Target        string `json:"target,omitempty"`
}

type ExecuteResponse struct {
	SessionID  string `json:"session_id"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Summary    string `json:"summary"`
	DurationMS int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
}

func (e *Executor) Run(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	sess, err := e.sessionStore.Get(ctx, req.SessionID)
	if err != nil {
		return ExecuteResponse{}, fmt.Errorf("session error: %w", err)
	}

	if sess.Status != session.StatusOpen {
		return ExecuteResponse{}, fmt.Errorf("session is not open")
	}

	if req.CWD == "" {
		req.CWD = sess.Workspace
	}

	act, err := e.actionStore.Create(ctx, actionpkg.Action{
		SessionID:     req.SessionID,
		Type:          "command.exec",
		Input:         req.Command,
		Status:        "pending",
		Metadata:      "{}",
		Title:         req.Title,
		Purpose:       req.Purpose,
		ReasoningStep: req.ReasoningStep,
		Target:        firstNonEmpty(req.Target, req.Command),
	})
	if err != nil {
		return ExecuteResponse{}, fmt.Errorf("create action: %w", err)
	}

	result := e.runner.Run(ctx, req.SessionID, req.Command, req.CWD, req.Timeout)

	const maxOutputSize = 10000
	if len(result.Output) > maxOutputSize {
		result.Output = result.Output[:maxOutputSize] + "\n...[truncated]"
	}

	if len(result.ErrorOutput) > maxOutputSize {
		result.ErrorOutput = result.ErrorOutput[:maxOutputSize] + "\n...[truncated]"
	}

	status := "success"
	if result.ExitCode != 0 {
		status = "failed"
	}

	combinedOutput := result.Output
	if result.ErrorOutput != "" {
		combinedOutput += "\n" + result.ErrorOutput
	}

	if err := e.actionStore.UpdateResult(ctx, act.ID, combinedOutput, status); err != nil {
		return ExecuteResponse{}, fmt.Errorf("update action result: %w", err)
	}

	if _, err := e.logStore.Create(ctx, logging.Log{
		SessionID: req.SessionID,
		ActionID:  act.ID,
		Level:     "info",
		Message:   "command executed",
		Metadata: actionpkg.MergeMetadata("{}", actionpkg.ActionMetadata{
			Title:         req.Title,
			Purpose:       req.Purpose,
			ReasoningStep: req.ReasoningStep,
			Target:        firstNonEmpty(req.Target, req.Command),
		}),
	}); err != nil {
		return ExecuteResponse{}, fmt.Errorf("create execution log: %w", err)
	}

	_, _ = e.commandStore.Create(ctx, command.CommandLog{
		SessionID:   req.SessionID,
		Command:     req.Command,
		CWD:         req.CWD,
		ExitCode:    result.ExitCode,
		Output:      result.Output,
		ErrorOutput: result.ErrorOutput,
		DurationMS:  result.DurationMS,
	})

	return ExecuteResponse{
		SessionID:  req.SessionID,
		Command:    req.Command,
		ExitCode:   result.ExitCode,
		Output:     result.Output,
		Summary:    summarize(result.Output),
		DurationMS: result.DurationMS,
		Success:    result.ExitCode == 0,
	}, nil
}

func summarize(output string) string {
	lines := strings.Split(output, "\n")

	if len(lines) > 3 {
		return strings.Join(lines[:3], "\n")
	}

	return output
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
