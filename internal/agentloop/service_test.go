package agentloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	storepkg "github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/tasks"
)

type stubBrain struct {
	reply string
	err   error
}

func (s stubBrain) Chat(ctx context.Context, messages []brain.Message) (string, error) {
	return s.reply, s.err
}

func (s stubBrain) Timeout() string {
	return "180s"
}

type sequenceBrain struct {
	replies []string
	calls   [][]brain.Message
}

func (s *sequenceBrain) Chat(ctx context.Context, messages []brain.Message) (string, error) {
	s.calls = append(s.calls, messages)
	if len(s.replies) == 0 {
		return `{"action":"explain","reason":"done"}`, nil
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	return reply, nil
}

func newTestService(t *testing.T, brainClient BrainClient, registry *ToolRegistry, limits LoopLimits) (*Service, *Store, *session.Store) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "ptolemy.db")
	baseStore, err := storepkg.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = baseStore.Close() })

	if err := storepkg.RunMigrations(t.Context(), baseStore.SQLDB()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	runStore := NewStore(baseStore.SQLDB())
	actionStore := action.NewStore(baseStore.SQLDB())
	logStore := logging.NewStore(baseStore.SQLDB())
	sessionStore := session.NewStore(baseStore)
	approvalStore := approval.NewStore(baseStore.SQLDB())
	if registry == nil {
		registry = NewToolRegistry()
		NewToolExecutor(sessionStore, approvalStore, nil, t.TempDir()).RegisterAll(registry)
	}

	service := NewService(
		runStore,
		actionStore,
		logStore,
		sessionStore,
		brainClient,
		registry,
		limits,
		nil,
	)

	return service, runStore, sessionStore
}

func TestStartOrResumeUsesRunWorkspaceForExecution(t *testing.T) {
	workspace := t.TempDir()
	taskDir := filepath.Join(workspace, "docs", "tasks", "inbox")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}

	taskPath := filepath.Join(taskDir, "test-task.md")
	taskContent := `---
priority: normal
task_id: test-task
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/test-task
allowed_files:
  - README.md
validation:
  - go test ./...
created_by: codex
---
# Test Task
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	var capturedWorkspace string
	registry := NewToolRegistry()
	registry.Register("explain", func(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
		capturedWorkspace = task.Workspace
		return ExecutionResult{
			Status:         "success",
			Output:         "done",
			Summary:        "done",
			Progressed:     true,
			ShouldContinue: false,
			ShouldFinalize: true,
		}, nil
	})

	service, runStore, sessionStore := newTestService(t, stubBrain{reply: `{"action":"explain","reason":"done"}`}, registry, DefaultLimits())
	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:       "test-task",
		TaskFile:     filepath.ToSlash(filepath.Join("docs", "tasks", "inbox", "test-task.md")),
		Workspace:    workspace,
		Branch:       "ptolemy/test-task",
		Status:       StatusPending,
		CurrentPhase: "task_runner",
		MaxSteps:     1,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	updated, err := service.StartOrResume(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("start or resume: %v", err)
	}

	if updated.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %q", updated.Status)
	}
	if capturedWorkspace != workspace {
		t.Fatalf("expected action workspace %q, got %q", workspace, capturedWorkspace)
	}
	if updated.SessionID == "" {
		t.Fatal("expected session to be created")
	}

	sess, err := sessionStore.Get(t.Context(), updated.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Workspace != workspace {
		t.Fatalf("expected session workspace %q, got %q", workspace, sess.Workspace)
	}
}

func TestStartOrResumeBootstrapsTerminalSession(t *testing.T) {
	workspace := t.TempDir()
	taskPath := filepath.Join(workspace, "task.md")
	taskContent := `---
priority: normal
task_id: bootstrap-session
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/bootstrap-session
allowed_files:
  - README.md
validation:
  - go test ./...
created_by: codex
---
# Bootstrap Session
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	service, runStore, _ := newTestService(t, stubBrain{reply: `{"action":"explain","reason":"done"}`}, nil, DefaultLimits())

	var gotSessionID string
	var gotWorkspace string
	service.SetSessionBootstrap(func(ctx context.Context, sessionID string, workspace string) error {
		gotSessionID = sessionID
		gotWorkspace = workspace
		return nil
	})

	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:    "bootstrap-session",
		TaskFile:  taskPath,
		Workspace: workspace,
		Branch:    "ptolemy/bootstrap-session",
		Status:    StatusPending,
		MaxSteps:  1,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	updated, err := service.StartOrResume(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("start or resume: %v", err)
	}

	if gotSessionID == "" {
		t.Fatal("expected session bootstrap to receive a session ID")
	}
	if gotSessionID != updated.SessionID {
		t.Fatalf("expected bootstrap session ID %q, got %q", updated.SessionID, gotSessionID)
	}
	if gotWorkspace != workspace {
		t.Fatalf("expected bootstrap workspace %q, got %q", workspace, gotWorkspace)
	}
}

func TestStartOrResumeFailsAfterInvalidJSONLimit(t *testing.T) {
	workspace := t.TempDir()
	taskPath := filepath.Join(workspace, "task.md")
	taskContent := `---
priority: normal
task_id: invalid-json
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/invalid-json
allowed_files:
  - README.md
validation:
  - go test ./...
created_by: codex
---
# Invalid JSON
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	service, runStore, _ := newTestService(t, stubBrain{reply: `{"action":"read_file"}{"action":"explain"}`}, NewToolRegistry(), DefaultLimits())
	service.limits.MaxInvalidJSON = 1

	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:    "invalid-json",
		TaskFile:  taskPath,
		Workspace: workspace,
		Branch:    "ptolemy/invalid-json",
		Status:    StatusPending,
		MaxSteps:  2,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	updated, err := service.StartOrResume(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("start or resume: %v", err)
	}

	if updated.Status != StatusFailed {
		t.Fatalf("expected failed status, got %q", updated.Status)
	}
	if updated.CurrentPhase != "invalid_model_output" {
		t.Fatalf("expected invalid_model_output phase, got %q", updated.CurrentPhase)
	}
	if updated.InvalidJSONCount != 1 {
		t.Fatalf("expected invalid JSON count 1, got %d", updated.InvalidJSONCount)
	}

	observations, err := runStore.ListObservations(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if observations[0].Source != "brain.invalid_json" {
		t.Fatalf("expected invalid JSON observation source, got %q", observations[0].Source)
	}
}

func TestStartOrResumeUsesFreshContextForEachChildTask(t *testing.T) {
	workspace := t.TempDir()
	taskPath := filepath.Join(workspace, "task.md")
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	taskContent := `---
priority: normal
task_id: feature-pack
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/feature-pack
allowed_files:
  - README.md
  - main.go
validation:
  - go test ./...
created_by: codex
---
# Feature Pack

This is a full implementation with multiple phases.

## Inspect
Review README.md and summarize the current behavior.

## Implement
Update main.go and summarize the implementation result.
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	brainClient := &sequenceBrain{
		replies: []string{
			`{"action":"read_file","path":"README.md","reason":"inspect"}`,
			`{"action":"explain","reason":"inspect complete"}`,
			`{"action":"explain","reason":"implementation complete"}`,
		},
	}

	service, runStore, _ := newTestService(t, brainClient, nil, DefaultLimits())
	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:    "feature-pack",
		TaskFile:  taskPath,
		Workspace: workspace,
		Branch:    "ptolemy/feature-pack",
		Status:    StatusPending,
		MaxSteps:  6,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	updated, err := service.StartOrResume(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("start or resume: %v", err)
	}
	if updated.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %q", updated.Status)
	}
	if len(brainClient.calls) < 3 {
		t.Fatalf("expected multiple brain calls, got %d", len(brainClient.calls))
	}

	secondChildPrompt := brainClient.calls[len(brainClient.calls)-1][1].Content
	if strings.Contains(secondChildPrompt, "read file README.md") {
		t.Fatalf("expected child prompt to reset raw observations, got:\n%s", secondChildPrompt)
	}
	if !strings.Contains(secondChildPrompt, "001-inspect") {
		t.Fatalf("expected previous child summary in next child prompt, got:\n%s", secondChildPrompt)
	}
}

func TestStartOrResumeTreatsMissingRequiredFieldsAsInvalidModelOutput(t *testing.T) {
	workspace := t.TempDir()
	taskPath := filepath.Join(workspace, "task.md")
	taskContent := `---
priority: normal
task_id: missing-path
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/missing-path
allowed_files:
  - README.md
validation:
  - go test ./...
created_by: codex
---
# Missing Path
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	service, runStore, _ := newTestService(t, stubBrain{reply: `{"action":"read_file"}`}, NewToolRegistry(), DefaultLimits())
	service.limits.MaxInvalidJSON = 1

	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:    "missing-path",
		TaskFile:  taskPath,
		Workspace: workspace,
		Branch:    "ptolemy/missing-path",
		Status:    StatusPending,
		MaxSteps:  2,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	updated, err := service.StartOrResume(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("start or resume: %v", err)
	}

	if updated.Status != StatusFailed {
		t.Fatalf("expected failed status, got %q", updated.Status)
	}
	if updated.CurrentPhase != "invalid_model_output" {
		t.Fatalf("expected invalid_model_output phase, got %q", updated.CurrentPhase)
	}
	if updated.InvalidJSONCount != 1 {
		t.Fatalf("expected invalid JSON count 1, got %d", updated.InvalidJSONCount)
	}
	if updated.LastError != action.ErrMissingPath.Error() {
		t.Fatalf("expected last error %q, got %q", action.ErrMissingPath.Error(), updated.LastError)
	}

	observations, err := runStore.ListObservations(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if observations[0].Summary != action.ErrMissingPath.Error() {
		t.Fatalf("expected observation summary %q, got %q", action.ErrMissingPath.Error(), observations[0].Summary)
	}
}

func TestInspectRunReturnsPromptStateForProcessRun(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".ptolemy", "context"), 0o755); err != nil {
		t.Fatalf("mkdir context: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".ptolemy", "PTOLEMY.md"), []byte("Search first."), 0o644); err != nil {
		t.Fatalf("write PTOLEMY: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".ptolemy", "context", "project-map.md"), []byte("Main areas: internal/httpapi"), 0o644); err != nil {
		t.Fatalf("write project map: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("run page notes"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	taskPath := filepath.Join(workspace, "task.md")
	taskContent := `---
priority: normal
task_id: feature-pack
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/feature-pack
allowed_files:
  - README.md
  - main.go
validation:
  - go test ./...
created_by: codex
---
# Feature Pack

This is a full implementation with multiple phases.

## Inspect
Review README.md and summarize the current behavior.

## Implement
Update main.go and summarize the implementation result.
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	task, err := tasks.ParseTaskMarkdown(taskPath, []byte(taskContent))
	if err != nil {
		t.Fatalf("parse task: %v", err)
	}
	manifest := tasks.BuildProcessManifest(task)
	if len(manifest.ChildTasks) < 2 {
		t.Fatalf("expected at least 2 child tasks, got %d", len(manifest.ChildTasks))
	}
	paths, err := tasks.EnsureProcessFiles(workspace, manifest)
	if err != nil {
		t.Fatalf("ensure process files: %v", err)
	}

	manifest.Status = tasks.ProcessStatusRunning
	manifest.ChildTasks[0].Status = tasks.ProcessStatusDone
	manifest.ChildTasks[1].Status = tasks.ProcessStatusRunning
	manifest.CurrentChildTaskID = manifest.ChildTasks[1].ID
	summaryPath, err := tasks.WriteProcessSummary(paths, tasks.ProcessSummary{
		ChildTaskID:      manifest.ChildTasks[0].ID,
		Title:            manifest.ChildTasks[0].Title,
		Status:           tasks.ProcessStatusDone,
		ValidationResult: "success",
	})
	if err != nil {
		t.Fatalf("write process summary: %v", err)
	}
	manifest.ChildTasks[0].ResultSummaryPath = summaryPath
	if err := tasks.WriteProcessManifest(paths.ManifestPath, manifest); err != nil {
		t.Fatalf("write process manifest: %v", err)
	}
	if err := tasks.WriteProcessTodo(paths.TodoPath, manifest); err != nil {
		t.Fatalf("write process todo: %v", err)
	}

	service, runStore, _ := newTestService(t, stubBrain{}, nil, DefaultLimits())
	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:       task.ID,
		TaskFile:     taskPath,
		Workspace:    workspace,
		Branch:       task.Branch,
		Status:       StatusRunning,
		CurrentPhase: "child_task:" + manifest.ChildTasks[1].ID,
		MaxSteps:     8,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	inspection, err := service.InspectRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("inspect run: %v", err)
	}
	if inspection.WaitingOn != "brain_response" {
		t.Fatalf("expected waiting_on brain_response, got %q", inspection.WaitingOn)
	}
	if inspection.Prompt == nil {
		t.Fatal("expected prompt state")
	}
	if inspection.Prompt.CurrentTaskID != manifest.ChildTasks[1].ID {
		t.Fatalf("expected current child task %q, got %q", manifest.ChildTasks[1].ID, inspection.Prompt.CurrentTaskID)
	}
	if inspection.Prompt.BrainTimeout != "180s" {
		t.Fatalf("expected brain timeout 180s, got %q", inspection.Prompt.BrainTimeout)
	}
	if inspection.Prompt.EstimatedPromptTokens <= 0 {
		t.Fatalf("expected estimated prompt tokens > 0, got %d", inspection.Prompt.EstimatedPromptTokens)
	}
	if len(inspection.Prompt.PreviousChildSummaries) != 1 {
		t.Fatalf("expected 1 previous child summary, got %d", len(inspection.Prompt.PreviousChildSummaries))
	}
	if !strings.Contains(inspection.Prompt.PreviousChildSummaries[0], manifest.ChildTasks[0].ID) {
		t.Fatalf("expected previous child summary path to reference %q, got %q", manifest.ChildTasks[0].ID, inspection.Prompt.PreviousChildSummaries[0])
	}
	if len(inspection.Prompt.ManifestProgress) < 2 {
		t.Fatalf("expected manifest progress items, got %d", len(inspection.Prompt.ManifestProgress))
	}
	if len(inspection.Prompt.IncludedContextFiles) == 0 {
		t.Fatal("expected included context files to be populated")
	}
	if inspection.Prompt.RawPromptPreview == "" {
		t.Fatal("expected raw prompt preview to be populated")
	}
}

func TestInspectRunReportsValidationWaitStateForPendingValidationCommand(t *testing.T) {
	workspace := t.TempDir()
	taskPath := filepath.Join(workspace, "task.md")
	taskContent := `---
priority: normal
task_id: validation-check
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/validation-check
allowed_files:
  - README.md
validation:
  - go test ./...
created_by: codex
---
# Validation Check
`
	if err := os.WriteFile(taskPath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	service, runStore, _ := newTestService(t, stubBrain{}, nil, DefaultLimits())
	run, err := runStore.CreateRun(t.Context(), Run{
		TaskID:       "validation-check",
		TaskFile:     taskPath,
		Workspace:    workspace,
		Branch:       "ptolemy/validation-check",
		Status:       StatusRunning,
		CurrentPhase: "reasoning_loop",
		MaxSteps:     4,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	if _, err := service.actionStore.Create(t.Context(), action.Action{
		AgentRunID: run.ID,
		Type:       "run_command",
		Target:     "go test ./...",
		Status:     "pending",
	}); err != nil {
		t.Fatalf("create action: %v", err)
	}

	inspection, err := service.InspectRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("inspect run: %v", err)
	}
	if inspection.WaitingOn != "validation" {
		t.Fatalf("expected waiting_on validation, got %q", inspection.WaitingOn)
	}
	if inspection.LatestActionType != "run_command" {
		t.Fatalf("expected latest action type run_command, got %q", inspection.LatestActionType)
	}
}
