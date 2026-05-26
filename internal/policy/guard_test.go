package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type fakeRunner struct {
	calls int
	res   terminal.Result
}

func (f *fakeRunner) Run(_ context.Context, _ string, _ string, _ int) terminal.Result {
	f.calls++
	return f.res
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at)
		VALUES('s1','n','open','.','','x','x')`); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGuardedRunnerDenyDoesNotExecute(t *testing.T) {
	s := openTestStore(t)
	r := &fakeRunner{}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, err := g.Run(context.Background(), "s1", "cat ./.env", ".", 5, CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) {
		t.Fatalf("expected ErrDenied, got %v", err)
	}
	if r.calls != 0 {
		t.Fatalf("raw runner must not be called on deny, got %d", r.calls)
	}
}

func TestGuardedRunnerAllowExecutes(t *testing.T) {
	s := openTestStore(t)
	r := &fakeRunner{res: terminal.Result{ExitCode: 0}}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, err := g.Run(context.Background(), "s1", "go test ./...", ".", 5, CallOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 1 {
		t.Fatalf("expected one execution, got %d", r.calls)
	}
}

func TestGuardedRunnerAskThenConfirm(t *testing.T) {
	s := openTestStore(t)
	r := &fakeRunner{res: terminal.Result{ExitCode: 0}}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, err := g.Run(context.Background(), "s1", "git push origin main", ".", 5, CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ErrNeedsConfirmation, got %v", err)
	}
	if !g.approvals.Approve(needs.PendingID) {
		t.Fatalf("approve failed")
	}
	if _, err := g.Run(context.Background(), "s1", "git push origin main", ".", 5, CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed retry failed: %v", err)
	}
	if r.calls != 1 {
		t.Fatalf("expected one execution after confirmation, got %d", r.calls)
	}
}
