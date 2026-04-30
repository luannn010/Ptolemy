package main

import (
	"bytes"
	"os"
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
	root := createPackFixture(t)

	var out bytes.Buffer
	if err := runCLI([]string{"run", "--pack", root, "--workspace", "."}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Planned: task-a") || !strings.Contains(output, "Completed: task-b") {
		t.Fatalf("unexpected output: %s", output)
	}
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
