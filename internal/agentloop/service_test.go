package agentloop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	storepkg "github.com/luannn010/ptolemy/internal/store"
)

type stubBrain struct {
	reply string
	err   error
}

func (s stubBrain) Chat(ctx context.Context, messages []brain.Message) (string, error) {
	return s.reply, s.err
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
