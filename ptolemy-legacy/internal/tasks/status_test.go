package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateTaskStatusFileUpdatesFrontmatter(t *testing.T) {
	path := writeStatusTaskFile(t, `---
task_id: x
status: inbox
branch: ptolemy/x
---
body
`)

	if err := UpdateTaskStatusFile(path, StatusRunning); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: running") {
		t.Fatalf("unexpected file contents: %s", string(data))
	}
}

func TestUpdateTaskStatusFilePreservesBody(t *testing.T) {
	body := "body line 1\nstatus: should stay\n"
	path := writeStatusTaskFile(t, `---
task_id: x
status: inbox
branch: ptolemy/x
---
`+body)

	if err := UpdateTaskStatusFile(path, StatusCompleted); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(data), body) {
		t.Fatalf("body not preserved: %q", string(data))
	}
}

func TestUpdateTaskStatusFileRejectsInvalidStatus(t *testing.T) {
	path := writeStatusTaskFile(t, `---
task_id: x
status: inbox
branch: ptolemy/x
---
body
`)

	if err := UpdateTaskStatusFile(path, "weird"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTaskStatusFileErrorsWithoutFrontmatter(t *testing.T) {
	path := writeStatusTaskFile(t, "body only")
	if err := UpdateTaskStatusFile(path, StatusCompleted); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTaskStatusFileDoesNotTouchBodyStatusText(t *testing.T) {
	path := writeStatusTaskFile(t, `---
task_id: x
status: inbox
branch: ptolemy/x
---
body
status: leave me alone
`)

	if err := UpdateTaskStatusFile(path, StatusFailed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: leave me alone") {
		t.Fatalf("expected body status to remain, got %s", string(data))
	}
}

func writeStatusTaskFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
