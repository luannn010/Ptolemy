package tasks

import "testing"

func TestParseTaskMarkdown_AllFields(t *testing.T) {
	content := []byte(`---
priority: high
task_id: add-task-frontmatter-model
parent_task: multi-task-scheduler
owner: unassigned
status: inbox
branch: ptolemy/add-task-frontmatter-model
execution_group: sequential
depends_on:
  - task-a
  - task-b
allowed_files:
  - internal/tasks/task.go
  - internal/tasks/task_test.go
validation:
  - go test ./internal/tasks
---

# Task`)

	task, err := ParseTaskMarkdown("docs/tasks/inbox/01.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != "add-task-frontmatter-model" {
		t.Fatalf("unexpected task id: %s", task.ID)
	}
	if task.Priority != "high" || task.ExecutionGroup != "sequential" {
		t.Fatalf("unexpected defaults/fields: %+v", task)
	}
	if len(task.DependsOn) != 2 || len(task.AllowedFiles) != 2 || len(task.Validation) != 1 {
		t.Fatalf("unexpected list parsing: %+v", task)
	}
}

func TestParseTaskMarkdown_Defaults(t *testing.T) {
	content := []byte(`---
task_id: t1
status: inbox
branch: ptolemy/t1
allowed_files:
  - a.go
---
Body`)

	task, err := ParseTaskMarkdown("p.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Priority != "normal" {
		t.Fatalf("expected default priority normal, got %q", task.Priority)
	}
	if task.ExecutionGroup != "sequential" {
		t.Fatalf("expected default execution group sequential, got %q", task.ExecutionGroup)
	}
	if len(task.DependsOn) != 0 || len(task.Validation) != 0 {
		t.Fatalf("expected empty optional lists, got %+v", task)
	}
}

func TestParseTaskMarkdown_MissingTaskID(t *testing.T) {
	content := []byte(`---
status: inbox
branch: ptolemy/t1
allowed_files:
  - a.go
---
Body`)

	_, err := ParseTaskMarkdown("p.md", content)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseTaskMarkdown_MissingFrontmatter(t *testing.T) {
	content := []byte(`# No frontmatter`)
	_, err := ParseTaskMarkdown("p.md", content)
	if err == nil {
		t.Fatal("expected error")
	}
}
