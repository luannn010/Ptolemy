package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/fileops"
	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/luannn010/ptolemy/internal/worktree"
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

type fakeGitOps struct {
	pushes   int
	statuses int
}

func (f *fakeGitOps) Status(_ context.Context) gitops.Result                     { f.statuses++; return gitops.Result{} }
func (f *fakeGitOps) Diff(_ context.Context) gitops.Result                       { return gitops.Result{} }
func (f *fakeGitOps) Log(_ context.Context) gitops.Result                        { return gitops.Result{} }
func (f *fakeGitOps) CurrentBranch(_ context.Context) gitops.Result              { return gitops.Result{} }
func (f *fakeGitOps) CurrentCommitSHA(_ context.Context) gitops.Result           { return gitops.Result{} }
func (f *fakeGitOps) ChangedFiles(_ context.Context) gitops.Result               { return gitops.Result{} }
func (f *fakeGitOps) Checkout(_ context.Context, _ string) gitops.Result         { return gitops.Result{} }
func (f *fakeGitOps) CreateBranch(_ context.Context, _ string) gitops.Result     { return gitops.Result{} }
func (f *fakeGitOps) EnsureBranch(_ context.Context, _ string) gitops.Result     { return gitops.Result{} }
func (f *fakeGitOps) CreateOrResetBranchFrom(_ context.Context, _, _ string) gitops.Result {
	return gitops.Result{}
}
func (f *fakeGitOps) StageFiles(_ context.Context, _ []string) gitops.Result            { return gitops.Result{} }
func (f *fakeGitOps) CommitConventional(_ context.Context, _ string) gitops.Result      { return gitops.Result{} }
func (f *fakeGitOps) CommitStagedConventional(_ context.Context, _ string) gitops.Result { return gitops.Result{} }
func (f *fakeGitOps) MergeNoFF(_ context.Context, _ string) gitops.Result               { return gitops.Result{} }
func (f *fakeGitOps) Push(_ context.Context, _, _ string) gitops.Result {
	f.pushes++
	return gitops.Result{}
}
func (f *fakeGitOps) CreatePullRequest(_ context.Context, _, _, _, _ string) gitops.Result {
	return gitops.Result{}
}

func TestGuardedGit_AskOnPush(t *testing.T) {
	s := openTestStore(t)
	gops := &fakeGitOps{}
	g := NewGuardedGit(NewEngine(DefaultRuleset()), NewApprovals(), gops, "/repo", s.DB)

	_, err := g.Push(context.Background(), "s1", "origin", "main", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ask, got %v", err)
	}
	if gops.pushes != 0 {
		t.Fatalf("raw push must not run before approval")
	}

	g.core.approvals.Approve(needs.PendingID)
	if _, err := g.Push(context.Background(), "s1", "origin", "main", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed push failed: %v", err)
	}
	if gops.pushes != 1 {
		t.Fatalf("expected one push after confirm, got %d", gops.pushes)
	}
}

func TestGuardedGit_DefaultAskOnStatus(t *testing.T) {
	s := openTestStore(t)
	gops := &fakeGitOps{}
	g := NewGuardedGit(NewEngine(DefaultRuleset()), NewApprovals(), gops, "/repo", s.DB)

	_, err := g.Status(context.Background(), "s1", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected default ask on git.read, got %v", err)
	}
	if gops.statuses != 0 {
		t.Fatalf("raw status must not run before approval")
	}
}

type fakeWorktree struct {
	creates int
	removes int
}

func (f *fakeWorktree) Create(_ context.Context, _, _ string) worktree.Result {
	f.creates++
	return worktree.Result{}
}
func (f *fakeWorktree) AddExisting(_ context.Context, _, _ string) worktree.Result {
	return worktree.Result{}
}
func (f *fakeWorktree) Remove(_ context.Context, _ string) worktree.Result {
	f.removes++
	return worktree.Result{}
}
func (f *fakeWorktree) List(_ context.Context) worktree.Result { return worktree.Result{} }

func TestGuardedWorktree_AskOnCreate(t *testing.T) {
	s := openTestStore(t)
	w := &fakeWorktree{}
	g := NewGuardedWorktree(NewEngine(DefaultRuleset()), NewApprovals(), w, "/wt", s.DB)

	_, err := g.Create(context.Background(), "s1", "x", "main", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected default ask, got %v", err)
	}
	if w.creates != 0 {
		t.Fatalf("raw create must not run before approval")
	}

	g.core.approvals.Approve(needs.PendingID)
	if _, err := g.Create(context.Background(), "s1", "x", "main", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed create failed: %v", err)
	}
	if w.creates != 1 {
		t.Fatalf("expected one create after confirm, got %d", w.creates)
	}
}
