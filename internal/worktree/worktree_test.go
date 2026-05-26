package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupRepo(t *testing.T) string {
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

func TestCreateWorktree(t *testing.T) {
	repo := setupRepo(t)
	worktreeRoot := filepath.Join(t.TempDir(), "worktrees")

	manager := NewManager(repo, worktreeRoot)

	result := manager.Create(context.Background(), "task-one", "feature/task-one")

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Output)
	}

	if _, err := os.Stat(filepath.Join(worktreeRoot, "task-one")); err != nil {
		t.Fatalf("expected worktree to exist: %v", err)
	}
}

func TestListWorktrees(t *testing.T) {
	repo := setupRepo(t)
	worktreeRoot := filepath.Join(t.TempDir(), "worktrees")

	manager := NewManager(repo, worktreeRoot)

	create := manager.Create(context.Background(), "task-two", "feature/task-two")
	if !create.Success {
		t.Fatalf("create failed: %s", create.Output)
	}

	list := manager.List(context.Background())

	if !list.Success {
		t.Fatalf("list failed: %s", list.Output)
	}

	if !strings.Contains(list.Output, "task-two") {
		t.Fatalf("expected list to include task-two, got %q", list.Output)
	}
}

func TestRemoveWorktree(t *testing.T) {
	repo := setupRepo(t)
	worktreeRoot := filepath.Join(t.TempDir(), "worktrees")

	manager := NewManager(repo, worktreeRoot)

	create := manager.Create(context.Background(), "task-three", "feature/task-three")
	if !create.Success {
		t.Fatalf("create failed: %s", create.Output)
	}

	remove := manager.Remove(context.Background(), "task-three")
	if !remove.Success {
		t.Fatalf("remove failed: %s", remove.Output)
	}

	if _, err := os.Stat(filepath.Join(worktreeRoot, "task-three")); !os.IsNotExist(err) {
		t.Fatalf("expected worktree to be removed")
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir())
	res := m.Create(context.Background(), "", "")
	if res.Success {
		t.Fatalf("Create with empty name must fail")
	}
	if !strings.Contains(res.Output, "name is required") {
		t.Fatalf("expected 'name is required', got %q", res.Output)
	}
}

func TestAddExistingRejectsEmptyName(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir())
	res := m.AddExisting(context.Background(), "", "b")
	if res.Success || !strings.Contains(res.Output, "name is required") {
		t.Fatalf("expected name-required failure, got %q", res.Output)
	}
}

func TestAddExistingRequiresBranch(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir())
	res := m.AddExisting(context.Background(), "name", "")
	if res.Success || !strings.Contains(res.Output, "branch is required") {
		t.Fatalf("expected branch-required failure, got %q", res.Output)
	}
}

func TestRemoveRejectsEmptyName(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir())
	res := m.Remove(context.Background(), "")
	if res.Success || !strings.Contains(res.Output, "name is required") {
		t.Fatalf("expected name-required failure, got %q", res.Output)
	}
}

func TestAddExistingAttachesExistingBranch(t *testing.T) {
	repo := setupRepo(t)
	wtDir := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "branch", "preexisting")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("branch preexisting: %v\n%s", err, out)
	}
	m := NewManager(repo, wtDir)
	res := m.AddExisting(context.Background(), "att", "preexisting")
	if !res.Success {
		t.Fatalf("AddExisting failed: %s", res.Output)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"  hello world  ": "hello-world",
		"a/b\\c":          "a-b-c",
		"plain":           "plain",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Fatalf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellQuoteAndPath(t *testing.T) {
	if got := shellQuote("a'b"); !strings.Contains(got, `'\''`) {
		t.Fatalf("shellQuote must escape single quotes: %q", got)
	}
	if got := shellQuotePath("/p/q"); !strings.Contains(got, "/p/q") {
		t.Fatalf("shellQuotePath must contain original path: %q", got)
	}
}
