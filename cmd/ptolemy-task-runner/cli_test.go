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
