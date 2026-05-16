package tasks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlanPreviewReturnsOrderedTaskIDs(t *testing.T) {
	dir := t.TempDir()
	writeCLITask(t, dir, "b.md", "task-b", "inbox", "ptolemy/task-b", "parallel", nil, []string{"printf b"})
	writeCLITask(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	ids, validationErrs, err := BuildPlanPreview(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validationErrs) != 0 {
		t.Fatalf("unexpected validation errors: %+v", validationErrs)
	}
	if len(ids) != 2 || ids[0] != "task-a" || ids[1] != "task-b" {
		t.Fatalf("unexpected ids: %+v", ids)
	}
}

func TestBuildPlanPreviewReturnsValidationErrors(t *testing.T) {
	dir := t.TempDir()
	writeCLITask(t, dir, "bad.md", "task-a", "inbox", "ptolemy/task-a", "weird", nil, []string{"printf a"})

	_, validationErrs, err := BuildPlanPreview(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validationErrs) == 0 {
		t.Fatal("expected validation errors")
	}
}

func TestRunInboxSchedulerRunsTasks(t *testing.T) {
	dir := t.TempDir()
	writeCLITask(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	result := RunInboxScheduler(context.Background(), dir, "")
	if len(result.CompletedTaskIDs) != 1 || result.CompletedTaskIDs[0] != "task-a" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func writeCLITask(t *testing.T, dir string, name string, id string, status string, branch string, group string, deps []string, validation []string) string {
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
