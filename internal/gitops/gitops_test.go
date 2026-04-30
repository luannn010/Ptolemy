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

	run("init", "-b", "main")
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

func TestEnsureBranchCreatesBranchWithoutCheckout(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	before := strings.TrimSpace(git.run(context.Background(), "git branch --show-current").Output)
	result := git.EnsureBranch(context.Background(), "feature/ensure-branch")
	after := strings.TrimSpace(git.run(context.Background(), "git branch --show-current").Output)
	branchList := git.run(context.Background(), "git branch --list feature/ensure-branch")

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}
	if before != after {
		t.Fatalf("expected current branch to stay %q, got %q", before, after)
	}
	if !strings.Contains(branchList.Output, "feature/ensure-branch") {
		t.Fatalf("expected branch list to contain feature/ensure-branch, got %q", branchList.Output)
	}
}

func TestEnsureBranchSucceedsWhenBranchAlreadyExists(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	first := git.EnsureBranch(context.Background(), "feature/existing")
	second := git.EnsureBranch(context.Background(), "feature/existing")

	if !first.Success {
		t.Fatalf("expected first ensure to succeed, got %s", first.Output)
	}
	if !second.Success {
		t.Fatalf("expected second ensure to succeed, got %s", second.Output)
	}
	if !strings.Contains(second.Output, "branch already exists") {
		t.Fatalf("expected already-exists output, got %q", second.Output)
	}
}

func TestCreateOrResetBranchFrom(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	result := git.CreateOrResetBranchFrom(context.Background(), "feature/reset", "HEAD")
	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	branchList := git.run(context.Background(), "git branch --list feature/reset")
	if !strings.Contains(branchList.Output, "feature/reset") {
		t.Fatalf("expected branch list to contain feature/reset, got %q", branchList.Output)
	}
}

func TestMergeNoFF(t *testing.T) {
	repo := setupGitRepo(t)
	git := New(repo)

	_ = git.CreateBranch(context.Background(), "feature/merge-me")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = git.CommitConventional(context.Background(), "feat(test): add feature")
	_ = git.Checkout(context.Background(), "main")

	result := git.MergeNoFF(context.Background(), "feature/merge-me")
	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	diff := git.run(context.Background(), "git show --stat --oneline -1")
	if !strings.Contains(diff.Output, "Merge") && !strings.Contains(diff.Output, "feature.txt") {
		t.Fatalf("expected merge commit output, got %q", diff.Output)
	}
}

func TestCreatePullRequestUsesGHCLI(t *testing.T) {
	repo := setupGitRepo(t)
	fakeBin := t.TempDir()
	logPath := filepath.Join(fakeBin, "gh.log")
	scriptPath := filepath.Join(fakeBin, "gh")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" > " + shellQuote(logPath) + "\nprintf '%s\\n' 'https://example.com/pr/123'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))
	bodyFile := filepath.Join(repo, "pr.md")
	if err := os.WriteFile(bodyFile, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	git := New(repo)
	result := git.CreatePullRequest(context.Background(), "main", "feature/test", "PR title", bodyFile)
	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "pr create") || !strings.Contains(string(logData), "--head feature/test") {
		t.Fatalf("unexpected gh invocation: %s", string(logData))
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
