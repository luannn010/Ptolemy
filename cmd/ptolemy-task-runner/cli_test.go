package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luannn010/ptolemy/internal/tasks"
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
	root := createPackFixture(t)

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
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workspace := createCLIPackWorkspace(t)
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	tasksTestNow := func() time.Time { return time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC) }
	tasks.PackArchiveNowForTest(tasksTestNow)
	t.Cleanup(func() { tasks.PackArchiveNowForTest(time.Now) })

	root := createPackFixture(t)

	var out bytes.Buffer
	if err := runCLI([]string{"run", "--pack", root, "--workspace", workspace}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Planned: task-a") || !strings.Contains(output, "Completed: task-b") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, filepath.Join(workspace, "docs", "tasks", "packs", "done", "300426")) {
		t.Fatalf("unexpected output: %s", output)
	}
}

func createCLIPackWorkspace(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	fakeBin := t.TempDir()
	ghLogPath := filepath.Join(fakeBin, "gh.log")

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
	run("config", "user.email", "cli-pack@example.com")
	run("config", "user.name", "CLI Pack Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial")
	run("branch", "-M", "main")

	cmd := exec.Command("git", "init", "--bare", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, string(out))
	}
	run("remote", "add", "origin", remote)
	run("push", "-u", "origin", "main")

	ghScript := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"" + ghLogPath + "\"\n" +
		"printf 'https://example.com/pr/123\\n'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "gh"), []byte(ghScript), 0o755); err != nil {
		t.Fatal(err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+originalPath)

	return dir
}

func TestRunPlanCommandRejectsInboxAndPackTogether(t *testing.T) {
	root := createPackFixture(t)

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

func createPackFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
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
	return root
}
