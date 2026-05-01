package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPlanCommandPrintsExecutionPlan(t *testing.T) {
	dir := t.TempDir()
	writeCLITaskFile(t, dir, "b.md", "task-b", "inbox", "ptolemy/task-b", "parallel", nil, []string{"printf b"})
	writeCLITaskFile(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	var out bytes.Buffer
	if err := runCLI([]string{"plan", "--inbox", dir}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Execution plan:") || !strings.Contains(output, "1. task-a") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestRunSchedulerCommandPrintsCompletedTasks(t *testing.T) {
	dir := t.TempDir()
	writeCLITaskFile(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	var out bytes.Buffer
	if err := runCLI([]string{"run", "--inbox", dir, "--workspace", "."}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Planned: task-a") || !strings.Contains(output, "Completed: task-a") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestRunPlanCommandPrintsPackExecutionPlan(t *testing.T) {
	root, _ := createPackFixture(t)

	var out bytes.Buffer
	if err := runCLI([]string{"plan", "--pack", root}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Execution plan:") || !strings.Contains(output, "1. task-a") || !strings.Contains(output, "2. task-b") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestRunSchedulerCommandPrintsCompletedPackTasks(t *testing.T) {
	root, repo := createPackFixture(t)

	var out bytes.Buffer
	if err := runCLI([]string{"run", "--pack", root, "--workspace", repo}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Planned: task-a") || !strings.Contains(output, "Completed: task-b") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestRunPlanCommandRejectsInboxAndPackTogether(t *testing.T) {
	root, _ := createPackFixture(t)

	var out bytes.Buffer
	err := runCLI([]string{"plan", "--inbox", "docs/tasks/inbox", "--pack", root}, &out)
	if err == nil {
		t.Fatal("expected error")
	}
}

func writeCLITaskFile(t *testing.T, dir string, name string, id string, status string, branch string, group string, deps []string, validation []string) string {
	t.Helper()

	content := "---\n" +
		"task_id: " + id + "\n" +
		"status: " + status + "\n" +
		"branch: " + branch + "\n" +
		"priority: normal\n" +
		"execution_group: " + group + "\n" +
		"allowed_files:\n" +
		"  - internal/tasks/example.go\n"

	if len(deps) > 0 {
		content += "depends_on:\n"
		for _, dep := range deps {
			content += "  - " + dep + "\n"
		}
	}

	content += "validation:\n"
	for _, cmd := range validation {
		content += "  - " + cmd + "\n"
	}
	content += "---\nbody\n"

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func createPackFixture(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	repo, _ := setupPackRepoWithPublish(t)
	for _, dir := range []string{"scripts", "task-scripts", "snippets", "inbox"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "TASK_PLAN.md"), []byte("# Task Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Pack\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := "pack_id: cli-pack\n" +
		"name: CLI Pack\n" +
		"version: 1\n" +
		"created_by: test\n" +
		"entrypoint: TASK_PLAN.md\n" +
		"folders:\n" +
		"  inbox: inbox\n" +
		"  scripts: scripts\n" +
		"  task_scripts: task-scripts\n" +
		"  snippets: snippets\n" +
		"execution_mode: sequential_first\n" +
		"validation:\n" +
		"  - go test ./internal/tasks\n" +
		"rules:\n" +
		"  max_allowed_files: 8\n" +
		"  require_validation: true\n" +
		"  require_branch: true\n" +
		"  stop_on_failure: true\n"
	if err := os.WriteFile(filepath.Join(root, "PACK_MANIFEST.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	writeCLITaskFile(t, filepath.Join(root, "inbox"), "b.md", "task-b", "inbox", "ptolemy/task-b", "parallel", []string{"task-a"}, []string{"printf b"})
	writeCLITaskFile(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})
	createFeatureBranchCommit(t, repo, "ptolemy/task-a", "task-a.txt", "A\n")
	createFeatureBranchCommit(t, repo, "ptolemy/task-b", "task-b.txt", "B\n")
	return root, repo
}

func setupPackRepoWithPublish(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	fakeBin := t.TempDir()
	ghLogPath := filepath.Join(fakeBin, "gh.log")

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init repo")

	cmd := exec.Command("git", "init", "--bare", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, string(out))
	}

	runGit(t, dir, "remote", "add", "origin", remote)
	runGit(t, dir, "push", "-u", "origin", "main")

	ghScriptPath := filepath.Join(fakeBin, "gh")
	ghScript := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s\\n' \"$*\" > %s\nprintf '%%s\\n' 'https://example.com/pr/123'\n", shellQuoteForScript(ghLogPath))
	if err := os.WriteFile(ghScriptPath, []byte(ghScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))

	return dir, ghLogPath
}

func createFeatureBranchCommit(t *testing.T, dir string, branch string, filename string, content string) {
	t.Helper()
	runGit(t, dir, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", filename)
	runGit(t, dir, "commit", "-m", "feat(test): add "+filename)
	runGit(t, dir, "push", "-u", "origin", branch)
	runGit(t, dir, "checkout", "main")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func shellQuoteForScript(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
