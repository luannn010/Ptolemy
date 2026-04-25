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
