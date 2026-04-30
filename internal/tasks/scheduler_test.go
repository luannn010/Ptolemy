package tasks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnableNoDependencies(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{{ID: "a", Status: StatusInbox}}
	if len(RunnableTasks(tasks, state)) != 1 {
		t.Fatal("expected runnable task")
	}
}

func TestRunnableCompletedDependency(t *testing.T) {
	state := NewMemoryStateStore()
	state.Set("a", StatusCompleted)
	tasks := []Task{{ID: "b", Status: StatusInbox, DependsOn: []string{"a"}}}
	if len(RunnableTasks(tasks, state)) != 1 {
		t.Fatal("expected runnable task with completed dependency")
	}
}

func TestBlockedMissingDependency(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{{ID: "b", Status: StatusInbox, DependsOn: []string{"a"}}}
	if len(BlockedTasks(tasks, state)) != 1 {
		t.Fatal("expected blocked task")
	}
}

func TestBlockedStatusNotRunnable(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{{ID: "b", Status: StatusBlocked}}
	if len(RunnableTasks(tasks, state)) != 0 {
		t.Fatal("blocked status should not be runnable")
	}
}

func TestRunnablePreservesOrder(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{
		{ID: "a", Status: StatusInbox},
		{ID: "b", Status: StatusInbox},
	}
	got := RunnableTasks(tasks, state)
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatal("order not preserved")
	}
}

func TestSchedulerRunsValidTasksInDependencyOrder(t *testing.T) {
	dir := t.TempDir()
	writeSchedulerTask(t, dir, "b.md", "task-b", "inbox", "ptolemy/task-b", "sequential", []string{"task-a"}, []string{"printf b"})
	writeSchedulerTask(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	result := NewScheduler(dir, "").Run(context.Background())
	if result.FailedTaskID != "" || len(result.ValidationErrors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.PlannedTaskIDs) != 2 || result.PlannedTaskIDs[0] != "task-a" || result.PlannedTaskIDs[1] != "task-b" {
		t.Fatalf("unexpected plan order: %+v", result.PlannedTaskIDs)
	}
}

func TestSchedulerStopsIfValidationErrorsExist(t *testing.T) {
	dir := t.TempDir()
	writeSchedulerTask(t, dir, "bad.md", "task-a", "", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	result := NewScheduler(dir, "").Run(context.Background())
	if len(result.ValidationErrors) == 0 {
		t.Fatalf("expected validation errors, got %+v", result)
	}
}

func TestSchedulerMarksSuccessfulTaskCompleted(t *testing.T) {
	dir := t.TempDir()
	path := writeSchedulerTask(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"})

	result := NewScheduler(dir, "").Run(context.Background())
	if len(result.CompletedTaskIDs) != 1 || result.CompletedTaskIDs[0] != "task-a" {
		t.Fatalf("unexpected result: %+v", result)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: completed") {
		t.Fatalf("expected completed status, got %s", string(data))
	}
}

func TestSchedulerMarksFailedTaskFailed(t *testing.T) {
	dir := t.TempDir()
	path := writeSchedulerTask(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"exit 2"})

	result := NewScheduler(dir, "").Run(context.Background())
	if result.FailedTaskID != "task-a" {
		t.Fatalf("unexpected result: %+v", result)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: failed") {
		t.Fatalf("expected failed status, got %s", string(data))
	}
}

func TestSchedulerDoesNotRunTasksAfterFailure(t *testing.T) {
	dir := t.TempDir()
	path := writeSchedulerTask(t, dir, "b.md", "task-b", "inbox", "ptolemy/task-b", "sequential", nil, []string{"printf b"})
	writeSchedulerTask(t, dir, "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"exit 2"})

	result := NewScheduler(dir, "").Run(context.Background())
	if result.FailedTaskID != "task-a" {
		t.Fatalf("unexpected result: %+v", result)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: inbox") {
		t.Fatalf("expected second task not to run, got %s", string(data))
	}
}

func writeSchedulerTask(t *testing.T, dir string, name string, id string, status string, branch string, group string, deps []string, validation []string) string {
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
