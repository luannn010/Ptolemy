package agentloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/tasks"
)

type BrainClient interface {
	Chat(ctx context.Context, messages []brain.Message) (string, error)
}

type Service struct {
	runStore         *Store
	actionStore      *action.Store
	logStore         *logging.Store
	sessionStore     *session.Store
	brain            BrainClient
	registry         *ToolRegistry
	limits           LoopLimits
	finalizer        *Finalizer
	sessionBootstrap func(ctx context.Context, sessionID string, workspace string) error
}

func NewService(
	runStore *Store,
	actionStore *action.Store,
	logStore *logging.Store,
	sessionStore *session.Store,
	brainClient BrainClient,
	registry *ToolRegistry,
	limits LoopLimits,
	finalizer *Finalizer,
) *Service {
	return &Service{
		runStore:     runStore,
		actionStore:  actionStore,
		logStore:     logStore,
		sessionStore: sessionStore,
		brain:        brainClient,
		registry:     registry,
		limits:       limits,
		finalizer:    finalizer,
	}
}

func (s *Service) SetSessionBootstrap(fn func(ctx context.Context, sessionID string, workspace string) error) {
	s.sessionBootstrap = fn
}

func (s *Service) StartOrResume(ctx context.Context, runID string) (Run, error) {
	run, err := s.runStore.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if s.brain == nil || s.registry == nil {
		return run, nil
	}

	task, err := s.loadTask(run)
	if err != nil {
		run.Status = StatusFailed
		run.LastError = err.Error()
		return s.runStore.UpdateRun(ctx, run)
	}

	run, err = s.ensureSession(ctx, run, task)
	if err != nil {
		return Run{}, err
	}

	return s.executeTaskRun(ctx, run, task)
}

func (s *Service) ensureSession(ctx context.Context, run Run, task tasks.Task) (Run, error) {
	if strings.TrimSpace(run.SessionID) != "" {
		return run, nil
	}

	workspace := runWorkspace(run, task)
	if workspace == "" {
		return run, fmt.Errorf("workspace is required")
	}

	sess, err := s.sessionStore.Create(ctx, session.CreateSessionRequest{
		Name:        "agent-run-" + task.ID,
		Workspace:   workspace,
		Description: "created by workerd agentloop",
	})
	if err != nil {
		return run, err
	}

	run.SessionID = sess.ID
	run.Workspace = workspace
	if s.sessionBootstrap != nil {
		if err := s.sessionBootstrap(ctx, run.SessionID, workspace); err != nil {
			return run, err
		}
	}
	return s.runStore.UpdateRun(ctx, run)
}

func (s *Service) loadTask(run Run) (tasks.Task, error) {
	if strings.TrimSpace(run.TaskFile) == "" {
		return tasks.Task{
			ID:           run.TaskID,
			Branch:       run.Branch,
			AllowedFiles: []string{},
			Validation:   []string{},
			Path:         run.TaskFile,
			Body:         run.TaskID,
		}, nil
	}

	taskFilePath := resolveTaskFilePath(run)
	data, err := os.ReadFile(taskFilePath)
	if err != nil {
		return tasks.Task{}, err
	}
	task, err := tasks.ParseTaskMarkdown(taskFilePath, data)
	if err != nil {
		return tasks.Task{}, err
	}
	if run.TaskID != "" {
		task.ID = run.TaskID
	}
	if run.Branch != "" {
		task.Branch = run.Branch
	}
	task.Path = taskFilePath
	return task, nil
}

func resolveTaskFilePath(run Run) string {
	taskFile := strings.TrimSpace(run.TaskFile)
	if taskFile == "" || filepath.IsAbs(taskFile) {
		return taskFile
	}

	workspace := strings.TrimSpace(run.WorktreePath)
	if workspace == "" {
		workspace = strings.TrimSpace(run.Workspace)
	}
	if workspace == "" {
		return taskFile
	}

	return filepath.Join(workspace, taskFile)
}

func runWorkspace(run Run, task tasks.Task) string {
	if strings.TrimSpace(run.WorktreePath) != "" {
		return run.WorktreePath
	}
	if strings.TrimSpace(run.Workspace) != "" {
		return run.Workspace
	}
	if strings.TrimSpace(task.Path) != "" {
		return filepath.Dir(task.Path)
	}
	return ""
}

func summarizeInvalidJSON(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, action.ErrMultipleObjects.Error()):
		return "multiple JSON objects returned"
	case strings.Contains(msg, action.ErrJSONArray.Error()):
		return "top-level JSON array returned"
	default:
		return msg
	}
}

func (s *Service) brainTimeoutString() string {
	type timeoutReporter interface {
		Timeout() string
	}
	if reporter, ok := s.brain.(timeoutReporter); ok {
		return reporter.Timeout()
	}
	return "unknown"
}
