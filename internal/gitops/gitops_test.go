package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", ".")
	run("commit", "-m", "chore: initial commit")

	return dir
}

func TestGitStatus(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := git.Status(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	if !strings.Contains(result.Output, "?? new.txt") {
		t.Fatalf("expected status to include new.txt, got %q", result.Output)
	}
}

func TestGitDiff(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := git.Diff(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	if !strings.Contains(result.Output, "changed") {
		t.Fatalf("expected diff to include changed, got %q", result.Output)
	}
}

func TestGitLog(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	result := git.Log(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	if !strings.Contains(result.Output, "chore: initial commit") {
		t.Fatalf("expected log to contain initial commit, got %q", result.Output)
	}
}

func TestCreateBranch(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	result := git.CreateBranch(context.Background(), "feature/test-branch")

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	current := git.run(context.Background(), "git branch --show-current")
	if strings.TrimSpace(current.Output) != "feature/test-branch" {
		t.Fatalf("expected branch feature/test-branch, got %q", current.Output)
	}
}

func TestCommitConventional(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := git.CommitConventional(context.Background(), "feat(worker): add gitops")

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	log := git.Log(context.Background())
	if !strings.Contains(log.Output, "feat(worker): add gitops") {
		t.Fatalf("expected commit in log, got %q", log.Output)
	}
}

func TestRejectInvalidConventionalCommit(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	result := git.CommitConventional(context.Background(), "bad message")

	if result.Success {
		t.Fatal("expected invalid commit message to fail")
	}
}
func TestGitStatusExcludesGeneratedFiles(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	if err := os.MkdirAll(filepath.Join(repo, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(repo, "state"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "bin", "workerd"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "state", "ptolemy.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := git.Status(context.Background())

	if strings.Contains(result.Output, "bin/workerd") {
		t.Fatalf("expected status to exclude bin/workerd, got %q", result.Output)
	}

	if strings.Contains(result.Output, "state/ptolemy.db") {
		t.Fatalf("expected status to exclude state/ptolemy.db, got %q", result.Output)
	}
}
