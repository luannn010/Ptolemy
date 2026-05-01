package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTaskInputBuildsPackAwareContract(t *testing.T) {
	root := t.TempDir()
	packRoot := filepath.Join(root, "docs", "tasks", "packs", "example-pack")

	for _, dir := range []string{
		filepath.Join(packRoot, "inbox"),
		filepath.Join(packRoot, "scripts"),
		filepath.Join(packRoot, "task-scripts"),
		filepath.Join(packRoot, "snippets"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	mustWriteTestFile(t, filepath.Join(packRoot, "README.md"), "# Example Pack\n")
	mustWriteTestFile(t, filepath.Join(packRoot, "TASK_PLAN.md"), "# Plan\n")
	mustWriteTestFile(t, filepath.Join(packRoot, "PACK_MANIFEST.yaml"), `pack_id: example-pack
name: Example Pack
version: 1
created_by: test
entrypoint: TASK_PLAN.md
workspace_root: .
default_max_steps: 6
folders:
  inbox: inbox
  scripts: scripts
  task_scripts: task-scripts
  snippets: snippets
execution_mode: sequential_first
`)
	mustWriteTestFile(t, filepath.Join(packRoot, "task-scripts", "step.md"), "# step script\nexact instructions\n")
	mustWriteTestFile(t, filepath.Join(packRoot, "snippets", "example.txt"), "snippet body\n")

	taskPath := filepath.Join(packRoot, "inbox", "04-example.md")
	taskMarkdown := `---
priority: normal
task_id: 04-example
owner: unassigned
status: inbox
branch: ptolemy/04-example
execution_group: sequential
max_steps: 6
requires_approval: false
stop_on_error: true
allowed_files:
  - internal/example/
validation:
  - go test ./internal/example/...
scripts:
  - task-scripts/step.md
snippets:
  - snippets/example.txt
created_by: test
---

# Task: Example

## Goal
Do the thing.

## Scope
Only allowed files.

## Constraints
- Keep changes small.

## Inputs
- ` + "`task-scripts/step.md`" + `
- ` + "`snippets/example.txt`" + `

## Execution Steps
1. Read inputs.

## Acceptance Checks
- validation passes

## Failure / Escalation
- stop if blocked

## Done when
- [ ] complete
`
	mustWriteTestFile(t, taskPath, taskMarkdown)

	data, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}

	got, detectedPackRoot, err := loadTaskInput(taskPath, data)
	if err != nil {
		t.Fatalf("loadTaskInput: %v", err)
	}
	if filepath.Clean(detectedPackRoot) != filepath.Clean(packRoot) {
		t.Fatalf("unexpected pack root: %q", detectedPackRoot)
	}

	if !strings.Contains(got, "# Ptolemy Task Execution Contract") {
		t.Fatalf("expected execution contract, got %q", got)
	}
	if !strings.Contains(got, "exact instructions") {
		t.Fatalf("expected task script contents in contract, got %q", got)
	}
	if !strings.Contains(got, "snippet body") {
		t.Fatalf("expected snippet contents in contract, got %q", got)
	}
}

func TestResolvePackReferencePath(t *testing.T) {
	packRoot := "/tmp/example-pack"

	got, ok := resolvePackReferencePath(packRoot, "task-scripts/step.md")
	if !ok {
		t.Fatal("expected task-scripts path to resolve")
	}
	if got != "/tmp/example-pack/task-scripts/step.md" {
		t.Fatalf("resolved path = %q", got)
	}

	if _, ok := resolvePackReferencePath(packRoot, "internal/example/file.go"); ok {
		t.Fatal("did not expect non-pack path to resolve")
	}
}

func mustWriteTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
