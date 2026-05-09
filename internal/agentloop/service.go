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
	"github.com/luannn010/ptolemy/internal/navigator"
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

	run.Status = StatusRunning
	run.CurrentPhase = "reasoning_loop"
	run.MaxSteps = s.limits.WithMaxSteps(run.MaxSteps).MaxSteps
	run, err = s.runStore.UpdateRun(ctx, run)
	if err != nil {
		return Run{}, err
	}

	for step := run.CurrentStep + 1; step <= run.MaxSteps; step++ {
		run.CurrentStep = step
		observations, err := s.runStore.ListObservations(ctx, run.ID)
		if err != nil {
			return Run{}, err
		}

		workspace := runWorkspace(run, task)
		reply, err := s.brain.Chat(ctx, BuildMessages(task, s.readKnowledgeBase(workspace), observations))
		if err != nil {
			run.Status = StatusFailed
			run.LastError = err.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		envelope, err := action.ValidateSingleJSONAction(reply)
		if err != nil {
			run.InvalidJSONCount++
			run.LastError = err.Error()
			_, _ = s.runStore.CreateObservation(ctx, Observation{
				RunID:      run.ID,
				Step:       step,
				Source:     "brain.invalid_json",
				Summary:    summarizeInvalidJSON(err),
				RawPayload: reply,
			})
			if run.InvalidJSONCount >= s.limits.MaxInvalidJSON {
				run.Status = StatusFailed
				run.CurrentPhase = "invalid_model_output"
				return s.runStore.UpdateRun(ctx, run)
			}
			if _, updateErr := s.runStore.UpdateRun(ctx, run); updateErr != nil {
				return Run{}, updateErr
			}
			continue
		}

		taskScope := ActionTask{
			ID:           task.ID,
			Branch:       task.Branch,
			AllowedFiles: task.AllowedFiles,
			Validation:   task.Validation,
			Workspace:    workspace,
		}

		recordedAction, err := s.actionStore.Create(ctx, action.Action{
			AgentRunID:    run.ID,
			SessionID:     run.SessionID,
			Type:          envelope.Action,
			Input:         marshalJSON(envelope),
			Status:        "pending",
			Metadata:      "{}",
			Title:         envelope.Title,
			Purpose:       envelope.Purpose,
			ReasoningStep: envelope.ReasoningStep,
			Target:        firstNonEmpty(envelope.Target, envelope.Path, envelope.Command),
		})
		if err != nil {
			run.Status = StatusFailed
			run.LastError = err.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		result, execErr := s.registry.Execute(ctx, run, taskScope, envelope)
		if execErr != nil {
			_ = s.actionStore.UpdateResult(ctx, recordedAction.ID, execErr.Error(), "failed")
			run.Status = StatusFailed
			run.LastError = execErr.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		if !result.Progressed {
			run.NoProgressCount++
		} else {
			run.NoProgressCount = 0
		}

		_ = s.actionStore.UpdateResult(ctx, recordedAction.ID, result.Output, result.Status)
		_, _ = s.runStore.CreateObservation(ctx, Observation{
			RunID:        run.ID,
			ActionID:     recordedAction.ID,
			Step:         step,
			Source:       envelope.Action,
			Summary:      result.Summary,
			ArtifactPath: result.ArtifactPath,
			RawPayload:   result.Output,
		})

		if result.ShouldFinalize {
			run.CurrentPhase = "finalizing"
			if s.finalizer != nil && run.FinalizationMode != "none" {
				reportPath, finalErr := s.finalizer.Finalize(ctx, run, task)
				run.FinalReportPath = reportPath
				if finalErr != nil {
					run.Status = StatusFailed
					run.LastError = finalErr.Error()
					return s.runStore.UpdateRun(ctx, run)
				}
			}
			run.Status = StatusCompleted
			return s.runStore.UpdateRun(ctx, run)
		}

		if run.NoProgressCount >= s.limits.MaxNoProgressSteps {
			run.Status = StatusFailed
			run.LastError = "no progress limit reached"
			return s.runStore.UpdateRun(ctx, run)
		}

		if _, updateErr := s.runStore.UpdateRun(ctx, run); updateErr != nil {
			return Run{}, updateErr
		}

		if !result.ShouldContinue {
			run.Status = StatusPaused
			return s.runStore.UpdateRun(ctx, run)
		}
	}

	run.Status = StatusFailed
	run.LastError = "max steps reached"
	return s.runStore.UpdateRun(ctx, run)
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

func (s *Service) readKnowledgeBase(workspace string) []navigator.ContextFile {
	files, err := navigator.ReadContext(workspace)
	if err != nil {
		return nil
	}
	return files
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
