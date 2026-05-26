package policy

import (
	"context"
	"testing"
)

// These tests exercise the remaining guard methods that the original
// guard_test.go doesn't touch — Resolve, Search, ApplyPatch, ReplaceBlock,
// InsertAfter on GuardedFileOps; all read/mutation methods on GuardedGit;
// AddExisting/Remove/List on GuardedWorktree. They use a permissive ruleset
// so the gate returns Allow and the raw adapter is invoked, which lets us
// verify wiring without writing 30 separate intent-shape assertions.

func permissiveEngine() *Engine {
	// One catch-all allow rule lets every kind through.
	rs := Ruleset{Rules: []Rule{{ID: "test-allow-all", Contains: "", Effect: "", Reason: "test"}}}
	// Empty Contains never matches — so we add a rule that always matches via
	// a known substring used in every intent we build below: the literal
	// "test-call". To keep intents simple we instead use a small set of
	// permissive rules keyed on the program name.
	rs.Rules = []Rule{
		{ID: "allow-resolve", Contains: "resolve", Effect: "allow", Reason: "t"},
		{ID: "allow-read", Contains: "read", Effect: "allow", Reason: "t"},
		{ID: "allow-write", Contains: "write", Effect: "allow", Reason: "t"},
		{ID: "allow-list", Contains: "list", Effect: "allow", Reason: "t"},
		{ID: "allow-search", Contains: "search", Effect: "allow", Reason: "t"},
		{ID: "allow-patch", Contains: "patch", Effect: "allow", Reason: "t"},
		{ID: "allow-replace", Contains: "replace", Effect: "allow", Reason: "t"},
		{ID: "allow-insert", Contains: "insert", Effect: "allow", Reason: "t"},
		{ID: "allow-git", Contains: "git", Effect: "allow", Reason: "t"},
		{ID: "allow-gh", Contains: "gh", Effect: "allow", Reason: "t"},
		{ID: "allow-worktree", Contains: "worktree", Effect: "allow", Reason: "t"},
	}
	return NewEngine(rs)
}

func TestGuardedFileOps_AllMethodsInvokeRaw(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(permissiveEngine(), NewApprovals(), f, s.DB)

	ctx := context.Background()
	if _, err := g.Resolve(ctx, "s1", "p", CallOpts{}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := g.ReadFile(ctx, "s1", "p", CallOpts{}); err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := g.WriteFile(ctx, "s1", "p", "c", CallOpts{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := g.ListDirectory(ctx, "s1", "p", CallOpts{}); err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	if _, err := g.Search(ctx, "s1", "q", CallOpts{}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if err := g.ApplyPatch(ctx, "s1", "p", "n", CallOpts{}); err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}
	if err := g.ReplaceBlock(ctx, "s1", "p", "o", "n", CallOpts{}); err != nil {
		t.Fatalf("ReplaceBlock: %v", err)
	}
	if err := g.InsertAfter(ctx, "s1", "p", "m", "c", CallOpts{}); err != nil {
		t.Fatalf("InsertAfter: %v", err)
	}
	if f.reads != 1 || f.writes != 1 || f.lists != 1 {
		t.Fatalf("expected single invocation of each tracked op: %+v", f)
	}
}

func TestGuardedGit_AllMethodsInvokeRaw(t *testing.T) {
	s := openTestStore(t)
	gops := &fakeGitOps{}
	g := NewGuardedGit(permissiveEngine(), NewApprovals(), gops, "/repo", s.DB)

	ctx := context.Background()
	if _, err := g.Status(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if _, err := g.Diff(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if _, err := g.Log(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if _, err := g.CurrentBranch(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if _, err := g.CurrentCommitSHA(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("CurrentCommitSHA: %v", err)
	}
	if _, err := g.ChangedFiles(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	if _, err := g.Checkout(ctx, "s1", "b", CallOpts{}); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if _, err := g.CreateBranch(ctx, "s1", "b", CallOpts{}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := g.EnsureBranch(ctx, "s1", "b", CallOpts{}); err != nil {
		t.Fatalf("EnsureBranch: %v", err)
	}
	if _, err := g.CreateOrResetBranchFrom(ctx, "s1", "b", "main", CallOpts{}); err != nil {
		t.Fatalf("CreateOrResetBranchFrom: %v", err)
	}
	if _, err := g.StageFiles(ctx, "s1", []string{"a"}, CallOpts{}); err != nil {
		t.Fatalf("StageFiles: %v", err)
	}
	if _, err := g.CommitConventional(ctx, "s1", "feat: x", CallOpts{}); err != nil {
		t.Fatalf("CommitConventional: %v", err)
	}
	if _, err := g.CommitStagedConventional(ctx, "s1", "feat: x", CallOpts{}); err != nil {
		t.Fatalf("CommitStagedConventional: %v", err)
	}
	if _, err := g.MergeNoFF(ctx, "s1", "b", CallOpts{}); err != nil {
		t.Fatalf("MergeNoFF: %v", err)
	}
	if _, err := g.Push(ctx, "s1", "origin", "main", CallOpts{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if _, err := g.CreatePullRequest(ctx, "s1", "main", "feat", "title", "body.txt", CallOpts{}); err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if gops.pushes != 1 || gops.statuses != 1 {
		t.Fatalf("counters: %+v", gops)
	}
}

func TestGuardedWorktree_AllMethodsInvokeRaw(t *testing.T) {
	s := openTestStore(t)
	w := &fakeWorktree{}
	g := NewGuardedWorktree(permissiveEngine(), NewApprovals(), w, "/wt", s.DB)

	ctx := context.Background()
	if _, err := g.Create(ctx, "s1", "n", "b", CallOpts{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := g.AddExisting(ctx, "s1", "n", "b", CallOpts{}); err != nil {
		t.Fatalf("AddExisting: %v", err)
	}
	if _, err := g.Remove(ctx, "s1", "n", CallOpts{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := g.List(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if w.creates != 1 || w.removes != 1 {
		t.Fatalf("counters: %+v", w)
	}
}
