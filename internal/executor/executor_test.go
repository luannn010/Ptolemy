package executor

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	actionpkg "github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

func newTestExecutor(t *testing.T) (*Executor, *session.Store, *command.Store, *actionpkg.Store) {
	t.Helper()

	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	dbPath := t.TempDir() + "/test.db"

	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}

	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	if err := store.RunMigrations(context.Background(), baseStore.SQLDB()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	actionStore := actionpkg.NewStore(baseStore.SQLDB())
	logStore := logging.NewStore(baseStore.SQLDB())
	runner := terminal.NewTmuxRunner()

	exec := NewExecutor(sessionStore, commandStore, actionStore, logStore, runner)

	return exec, sessionStore, commandStore, actionStore
}

func TestExecutorRunSuccess(t *testing.T) {
	executor, sessionStore, _, _ := newTestExecutor(t)

	sess, err := sessionStore.Create(context.Background(), session.CreateSessionRequest{
		Name:      "executor-test",
		Workspace: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer terminal.KillSession(sess.ID)

	resp, err := executor.Run(context.Background(), ExecuteRequest{
		SessionID: sess.ID,
		Command:   "echo executor-ok",
		Timeout:   5,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success to be true")
	}

	if resp.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", resp.ExitCode)
	}

	if !strings.Contains(resp.Output, "executor-ok") {
		t.Fatalf("expected output to contain executor-ok, got %q", resp.Output)
	}
}

func TestExecutorRunFailure(t *testing.T) {
	executor, sessionStore, _, _ := newTestExecutor(t)

	sess, err := sessionStore.Create(context.Background(), session.CreateSessionRequest{
		Name:      "executor-failure-test",
		Workspace: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer terminal.KillSession(sess.ID)

	resp, err := executor.Run(context.Background(), ExecuteRequest{
		SessionID: sess.ID,
		Command:   "exit 5",
		Timeout:   5,
	})
	if err != nil {
		t.Fatalf("expected no executor error, got %v", err)
	}

	if resp.Success {
		t.Fatal("expected success to be false")
	}

	if resp.ExitCode != 5 {
		t.Fatalf("expected exit code 5, got %d", resp.ExitCode)
	}
}

func TestExecutorRejectsClosedSession(t *testing.T) {
	executor, sessionStore, _, _ := newTestExecutor(t)

	sess, err := sessionStore.Create(context.Background(), session.CreateSessionRequest{
		Name:      "executor-closed-test",
		Workspace: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	_, err = sessionStore.CloseSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	_, err = executor.Run(context.Background(), ExecuteRequest{
		SessionID: sess.ID,
		Command:   "echo should-not-run",
		Timeout:   5,
	})
	if err == nil {
		t.Fatal("expected error for closed session")
	}
}

func TestExecutorStoresCommandLog(t *testing.T) {
	executor, sessionStore, commandStore, _ := newTestExecutor(t)

	sess, err := sessionStore.Create(context.Background(), session.CreateSessionRequest{
		Name:      "executor-log-test",
		Workspace: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer terminal.KillSession(sess.ID)

	_, err = executor.Run(context.Background(), ExecuteRequest{
		SessionID: sess.ID,
		Command:   "echo log-ok",
		Timeout:   5,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	logs, err := commandStore.ListBySession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("failed to list command logs: %v", err)
	}

	if len(logs) != 1 {
		t.Fatalf("expected 1 command log, got %d", len(logs))
	}

	if logs[0].Command != "echo log-ok" {
		t.Fatalf("expected command log to store command, got %q", logs[0].Command)
	}
}

func TestExecutorStoresDescriptiveActionMetadata(t *testing.T) {
	executor, sessionStore, _, actionStore := newTestExecutor(t)

	sess, err := sessionStore.Create(context.Background(), session.CreateSessionRequest{
		Name:      "executor-metadata-test",
		Workspace: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer terminal.KillSession(sess.ID)

	_, err = executor.Run(context.Background(), ExecuteRequest{
		SessionID:     sess.ID,
		Command:       "echo metadata-log-ok",
		Timeout:       5,
		Title:         "Run metadata smoke test",
		Purpose:       "Confirm the execute path stores descriptive metadata.",
		ReasoningStep: "Validate runtime metadata",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	actions, err := actionStore.ListBySession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("failed to list actions: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	got := actions[0]
	if got.Title != "Run metadata smoke test" {
		t.Fatalf("expected title to round-trip, got %q", got.Title)
	}
	if got.Purpose != "Confirm the execute path stores descriptive metadata." {
		t.Fatalf("expected purpose to round-trip, got %q", got.Purpose)
	}
	if got.ReasoningStep != "Validate runtime metadata" {
		t.Fatalf("expected reasoning step to round-trip, got %q", got.ReasoningStep)
	}
	if got.Target != "echo metadata-log-ok" {
		t.Fatalf("expected target to default to command, got %q", got.Target)
	}
}
