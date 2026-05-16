package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanInbox_MultipleValidMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeTaskFile(t, dir, "a.md", "a", "high")
	writeTaskFile(t, dir, "b.md", "b", "normal")

	tasks, err := ScanInbox(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestScanInbox_IgnoresNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeTaskFile(t, dir, "a.md", "a", "normal")
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write ignore file: %v", err)
	}

	tasks, err := ScanInbox(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestScanInbox_SortsByPriorityThenFilename(t *testing.T) {
	dir := t.TempDir()
	writeTaskFile(t, dir, "c.md", "c", "low")
	writeTaskFile(t, dir, "b.md", "b", "high")
	writeTaskFile(t, dir, "a.md", "a", "high")
	writeTaskFile(t, dir, "d.md", "d", "weird-priority")

	tasks, err := ScanInbox(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := []string{
		filepath.Base(tasks[0].Path),
		filepath.Base(tasks[1].Path),
		filepath.Base(tasks[2].Path),
		filepath.Base(tasks[3].Path),
	}
	want := []string{"a.md", "b.md", "d.md", "c.md"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order: got %v, want %v", got, want)
		}
	}
}

func TestScanInbox_ReturnsCombinedErrorForInvalidTask(t *testing.T) {
	dir := t.TempDir()
	writeTaskFile(t, dir, "good.md", "good", "normal")
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("---\nstatus: inbox\n---\nbody"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	tasks, err := ScanInbox(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 valid task, got %d", len(tasks))
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bad.md") {
		t.Fatalf("expected error to mention bad filename, got %q", err.Error())
	}
}

func writeTaskFile(t *testing.T, dir string, name string, id string, priority string) {
	t.Helper()
	content := "---\n" +
		"task_id: " + id + "\n" +
		"status: inbox\n" +
		"branch: ptolemy/" + id + "\n" +
		"priority: " + priority + "\n" +
		"allowed_files:\n" +
		"  - x.go\n" +
		"---\n" +
		"body\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", name, err)
	}
}
