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
max_steps: 8
requires_approval: false
stop_on_error: true
---

# Task

## Goal
Ship the task frontmatter model.

## Scope
Only update the task model files.

## Constraints
Keep the change small.

## Inputs
- None.

## Execution Steps
1. Add the fields.

## Acceptance Checks
- Run go test.

## Failure / Escalation
- Stop if parsing becomes ambiguous.

## Done When
- Parsing works.`)

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
	if task.MaxSteps != 8 || task.RequiresApproval || !task.StopOnError {
		t.Fatalf("unexpected execution fields: %+v", task)
	}
	if task.Sections["Goal"] == "" || task.Sections["Done When"] == "" {
		t.Fatalf("expected parsed sections, got %+v", task.Sections)
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
	if len(task.Scripts) != 0 || len(task.Snippets) != 0 {
		t.Fatalf("expected empty pack lists, got %+v", task)
	}
	if task.StopOnError != true {
		t.Fatalf("expected default stop_on_error true, got %+v", task)
	}
}

func TestParseTaskMarkdown_InlineDependsOnList(t *testing.T) {
	content := []byte(`---
task_id: t1
status: inbox
branch: ptolemy/t1
depends_on: ['task-a', 'task-b']
allowed_files:
  - a.go
---
Body`)

	task, err := ParseTaskMarkdown("p.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(task.DependsOn) != 2 || task.DependsOn[0] != "task-a" || task.DependsOn[1] != "task-b" {
		t.Fatalf("unexpected depends_on parsing: %+v", task.DependsOn)
	}
}

func TestParseTaskMarkdown_PackReferences(t *testing.T) {
	content := []byte(`---
task_id: t1
status: inbox
branch: ptolemy/t1
allowed_files:
  - a.go
scripts:
  - task-scripts/01.md
snippets:
  - snippets/example.go
---
Body`)

	task, err := ParseTaskMarkdown("p.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(task.Scripts) != 1 || task.Scripts[0] != "task-scripts/01.md" {
		t.Fatalf("unexpected scripts: %+v", task.Scripts)
	}
	if len(task.Snippets) != 1 || task.Snippets[0] != "snippets/example.go" {
		t.Fatalf("unexpected snippets: %+v", task.Snippets)
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
