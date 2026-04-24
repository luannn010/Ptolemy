package executor

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

func newTestExecutor(t *testing.T) (*Executor, *session.Store, *command.Store) {
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

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	runner := terminal.NewTmuxRunner()

	exec := NewExecutor(sessionStore, commandStore, runner)

	return exec, sessionStore, commandStore
}

func TestExecutorRunSuccess(t *testing.T) {
	executor, sessionStore, _ := newTestExecutor(t)

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
	executor, sessionStore, _ := newTestExecutor(t)

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
	executor, sessionStore, _ := newTestExecutor(t)

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
	executor, sessionStore, commandStore := newTestExecutor(t)

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
