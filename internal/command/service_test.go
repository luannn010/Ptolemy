package command

import (
	"context"
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type fakeGuardedRunner struct {
	runResult terminal.Result
	runErr    error
	calls     int
}

func (f *fakeGuardedRunner) Run(_ context.Context, _ string, _ string, _ string, _ int, _ policy.CallOpts) (terminal.Result, error) {
	f.calls++
	return f.runResult, f.runErr
}

func newServiceHarness(t *testing.T) (*Service, *Store, *fakeGuardedRunner, *store.Store) {
	t.Helper()

	baseStore, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = baseStore.Close() })

	commandStore := NewStore(baseStore)
	runner := &fakeGuardedRunner{}
	svc := NewService(runner, commandStore)
	return svc, commandStore, runner, baseStore
}

func ensureSession(t *testing.T, base *store.Store, id string) {
	t.Helper()
	_, err := base.DB.Exec(
		`INSERT INTO sessions(id, name, status, workspace, description, created_at, updated_at)
		 VALUES(?, 'test', 'open', '.', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		id,
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func TestServiceRunAllowWritesCommandLog(t *testing.T) {
	svc, store, runner, base := newServiceHarness(t)
	ensureSession(t, base, "s1")
	runner.runResult = terminal.Result{ExitCode: 0, Output: "ok\n", DurationMS: 12}

	result, err := svc.Run(context.Background(), "s1", RunCommandRequest{
		Command: "go test ./...",
		CWD:     ".",
		Timeout: 30,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Log == nil {
		t.Fatalf("expected log")
	}
	if runner.calls != 1 {
		t.Fatalf("expected runner called once, got %d", runner.calls)
	}

	logs, err := store.ListBySession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("list logs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 command log, got %d", len(logs))
	}
}

func TestServiceRunAskReturnsConfirmationWithoutLog(t *testing.T) {
	svc, store, _, base := newServiceHarness(t)
	ensureSession(t, base, "s2")
	svc.runner = &fakeGuardedRunner{
		runErr: policy.ErrNeedsConfirmation{
			PendingID: "p1",
			Channel:   domain.ChannelOOB,
			Reason:    "needs confirmation",
		},
	}

	result, err := svc.Run(context.Background(), "s2", RunCommandRequest{Command: "git push origin main"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Confirmation == nil || result.Confirmation.PendingID != "p1" {
		t.Fatalf("expected confirmation payload")
	}
	logs, err := store.ListBySession(context.Background(), "s2")
	if err != nil {
		t.Fatalf("list logs failed: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no command log for ask path, got %d", len(logs))
	}
}

func TestServiceRunDenyReturnsErrorWithoutLog(t *testing.T) {
	svc, store, _, base := newServiceHarness(t)
	ensureSession(t, base, "s3")
	svc.runner = &fakeGuardedRunner{
		runErr: policy.ErrDenied{RuleID: "deny-secret-cmd", Reason: "denied"},
	}

	_, err := svc.Run(context.Background(), "s3", RunCommandRequest{Command: "cat ./.env"})
	if err == nil {
		t.Fatalf("expected deny error")
	}
	logs, err := store.ListBySession(context.Background(), "s3")
	if err != nil {
		t.Fatalf("list logs failed: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no command log for deny path, got %d", len(logs))
	}
}
