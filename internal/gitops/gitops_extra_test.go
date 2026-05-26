package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentBranchAndSHA(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	br := git.CurrentBranch(ctx)
	if !br.Success {
		t.Fatalf("CurrentBranch: %s", br.Output)
	}
	if strings.TrimSpace(br.Output) != "main" {
		t.Fatalf("expected main, got %q", br.Output)
	}

	sha := git.CurrentCommitSHA(ctx)
	if !sha.Success {
		t.Fatalf("CurrentCommitSHA: %s", sha.Output)
	}
	if len(strings.TrimSpace(sha.Output)) < 7 {
		t.Fatalf("expected commit SHA, got %q", sha.Output)
	}
}

func TestStageFilesAndCommitStaged(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if res := git.StageFiles(ctx, []string{"a.txt"}); !res.Success {
		t.Fatalf("StageFiles: %s", res.Output)
	}
	if res := git.CommitStagedConventional(ctx, "feat: add a"); !res.Success {
		t.Fatalf("CommitStagedConventional: %s", res.Output)
	}

	log := git.Log(ctx)
	if !log.Success || !strings.Contains(log.Output, "feat: add a") {
		t.Fatalf("expected new commit in log, got %q", log.Output)
	}
}

func TestChangedFiles(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := git.ChangedFiles(ctx)
	if !res.Success {
		t.Fatalf("ChangedFiles: %s", res.Output)
	}
	if !strings.Contains(res.Output, "README.md") {
		t.Fatalf("expected README.md in changed files, got %q", res.Output)
	}
}

func TestPushToLocalBareRepo(t *testing.T) {
	repo := setupGitRepo(t)
	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", repo, "remote", "add", "origin", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remote add: %v\n%s", err, out)
	}

	git := New(repo)
	res := git.Push(context.Background(), "origin", "main")
	if !res.Success {
		t.Fatalf("Push: %s", res.Output)
	}
}

func TestCommitConventionalCommitsAll(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := git.CommitConventional(ctx, "feat: add b"); !res.Success {
		t.Fatalf("CommitConventional: %s", res.Output)
	}
	if log := git.Log(ctx); !strings.Contains(log.Output, "feat: add b") {
		t.Fatalf("expected feat: add b in log, got %q", log.Output)
	}
}

func TestDiffShowsUnstagedChanges(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := git.Diff(ctx)
	if !res.Success {
		t.Fatalf("Diff: %s", res.Output)
	}
	if !strings.Contains(res.Output, "world") {
		t.Fatalf("expected diff to include new line, got %q", res.Output)
	}
}

func TestCreateBranchAndCheckout(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if res := git.CreateBranch(ctx, "feature/x"); !res.Success {
		t.Fatalf("CreateBranch: %s", res.Output)
	}
	if res := git.Checkout(ctx, "feature/x"); !res.Success {
		t.Fatalf("Checkout: %s", res.Output)
	}
	if res := git.CurrentBranch(ctx); strings.TrimSpace(res.Output) != "feature/x" {
		t.Fatalf("expected feature/x, got %q", res.Output)
	}
}

func TestEnsureBranchIdempotent(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if res := git.EnsureBranch(ctx, "feature/e"); !res.Success {
		t.Fatalf("first EnsureBranch: %s", res.Output)
	}
	if res := git.EnsureBranch(ctx, "feature/e"); !res.Success {
		t.Fatalf("second EnsureBranch must succeed: %s", res.Output)
	}
}

func TestMergeNoFFAcrossBranches(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)
	ctx := context.Background()

	if res := git.CreateBranch(ctx, "feature/m"); !res.Success {
		t.Fatalf("CreateBranch: %s", res.Output)
	}
	if res := git.Checkout(ctx, "feature/m"); !res.Success {
		t.Fatalf("Checkout: %s", res.Output)
	}
	if err := os.WriteFile(filepath.Join(repo, "c.txt"), []byte("c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := git.CommitConventional(ctx, "feat: c"); !res.Success {
		t.Fatalf("CommitConventional: %s", res.Output)
	}
	if res := git.Checkout(ctx, "main"); !res.Success {
		t.Fatalf("Checkout main: %s", res.Output)
	}
	if res := git.MergeNoFF(ctx, "feature/m"); !res.Success {
		t.Fatalf("MergeNoFF: %s", res.Output)
	}
}
