package agentloop

import (
	"context"
	"errors"

	"github.com/luannn010/ptolemy/internal/action"
)

var ErrUnknownAction = errors.New("unknown action")

type ExecutionResult struct {
	Status         string
	Output         string
	Summary        string
	ArtifactPath   string
	ApprovalID     string
	Progressed     bool
	ShouldContinue bool
	ShouldFinalize bool
}

type ActionExecutor func(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error)

type ActionTask struct {
	ID           string
	Branch       string
	AllowedFiles []string
	Validation   []string
	Workspace    string
}

type ToolRegistry struct {
	handlers map[string]ActionExecutor
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{handlers: map[string]ActionExecutor{}}
}

func (r *ToolRegistry) Register(actionType string, handler ActionExecutor) {
	r.handlers[actionType] = handler
}

func (r *ToolRegistry) Execute(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	handler, ok := r.handlers[envelope.Action]
	if !ok {
		return ExecutionResult{}, ErrUnknownAction
	}
	return handler(ctx, run, task, envelope)
}
