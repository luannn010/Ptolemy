package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/fileops"
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

type fakeFileOps struct {
	reads  int
	writes int
	lists  int
}

func (f *fakeFileOps) Resolve(path string) (string, error)                  { return path, nil }
func (f *fakeFileOps) ReadFile(path string) (string, error)                 { f.reads++; return "content", nil }
func (f *fakeFileOps) WriteFile(path, content string) error                 { f.writes++; return nil }
func (f *fakeFileOps) ListDirectory(path string) ([]fileops.DirEntry, error) {
	f.lists++
	return nil, nil
}
func (f *fakeFileOps) Search(query string) (string, error)                  { return "", nil }
func (f *fakeFileOps) ApplyPatch(path, newContent string) error             { return nil }
func (f *fakeFileOps) ReplaceBlock(path, oldS, newS string) error           { return nil }
func (f *fakeFileOps) InsertAfter(path, marker, content string) error       { return nil }

func TestGuardedFileOps_DenyPolicyWrite(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	err := g.WriteFile(context.Background(), "s1", ".ptolemy/policy.json", "{}", CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) || denied.RuleID != "deny-policy-write" {
		t.Fatalf("expected deny-policy-write, got %v", err)
	}
	if f.writes != 0 {
		t.Fatalf("raw adapter must not be called on deny")
	}
}

func TestGuardedFileOps_DenySecretRead(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	_, err := g.ReadFile(context.Background(), "s1", "./.env", CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) || denied.RuleID != "deny-secret-cmd" {
		t.Fatalf("expected deny-secret-cmd, got %v", err)
	}
	if f.reads != 0 {
		t.Fatalf("raw adapter must not be called on deny")
	}
}

func TestGuardedFileOps_AskThenConfirmList(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	_, err := g.ListDirectory(context.Background(), "s1", "/some/dir", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ErrNeedsConfirmation, got %v", err)
	}
	if !g.core.approvals.Approve(needs.PendingID) {
		t.Fatalf("approve failed")
	}
	if _, err := g.ListDirectory(context.Background(), "s1", "/some/dir", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed retry failed: %v", err)
	}
	if f.lists != 1 {
		t.Fatalf("expected exactly one list call after confirmation, got %d", f.lists)
	}
}

func TestGate_SwapIntentRejected(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	_, err := g.ListDirectory(context.Background(), "s1", "/a", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ask, got %v", err)
	}
	g.core.approvals.Approve(needs.PendingID)

	_, err = g.ListDirectory(context.Background(), "s1", "/b", CallOpts{ConfirmToken: needs.PendingID})
	if err == nil {
		t.Fatalf("expected swap-intent to be rejected, got nil")
	}
	if f.lists != 0 {
		t.Fatalf("raw must not be called on swap-intent")
	}
}
