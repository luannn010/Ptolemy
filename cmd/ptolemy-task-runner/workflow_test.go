package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowSmallTaskMovesThroughProcessToDoneAndArchive(t *testing.T) {
	chdirTemp(t)
	restore := stubRunAgent(t, []byte("agent ok\n"), nil)
	defer restore()

	writeTask(t, filepath.Join(inboxDir, "000-small.md"), "# Small\nCreate a file.")

	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Result: completed") {
		t.Fatalf("expected completed output, got %s", out.String())
	}
	assertMissing(t, filepath.Join(inboxDir, "000-small.md"))
	assertEmptyDir(t, activeDir)
	assertEmptyDir(t, processDir)
	assertExists(t, filepath.Join(doneDir, "000-small.md"))
	assertExists(t, filepath.Join(archiveDir, "000-small.md"))
	assertExists(t, filepath.Join(taskRunnerStateDir, "000-small-output.txt"))
}

func TestWorkflowFailedTaskMovesToFailedAndWritesNotification(t *testing.T) {
	chdirTemp(t)
	restore := stubRunAgent(t, []byte("agent failed\n"), errors.New("exit status 1"))
	defer restore()

	writeTask(t, filepath.Join(inboxDir, "000-fail.md"), "# Fail\nCreate a file.")

	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Result: failed") {
		t.Fatalf("expected failed output, got %s", out.String())
	}
	if !strings.Contains(out.String(), "Notification:") {
		t.Fatalf("expected notification output, got %s", out.String())
	}
	assertExists(t, filepath.Join(failedDir, "000-fail.md"))
	assertExists(t, filepath.Join(notificationDir, "000-fail-failed.txt"))
	assertMissing(t, filepath.Join(doneDir, "000-fail.md"))
}

func TestWorkflowLargeInboxTaskSplitsAndArchivesParent(t *testing.T) {
	chdirTemp(t)
	called := false
	previous := runAgent
	runAgent = func(taskPath string, maxSteps int) ([]byte, error) {
		called = true
		return []byte("should not run"), nil
	}
	t.Cleanup(func() {
		runAgent = previous
	})

	writeTask(t, filepath.Join(inboxDir, "000-large.md"), `# Large

Build a full task runner pipeline.

Requirements:
- scan docs/tasks/inbox
- split large tasks
- execute split tasks
`)

	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	if called {
		t.Fatal("large inbox task should split without running the agent")
	}
	if !strings.Contains(out.String(), "Result: split") {
		t.Fatalf("expected split output, got %s", out.String())
	}
	assertMissing(t, filepath.Join(inboxDir, "000-large.md"))
	assertEmptyDir(t, activeDir)
	assertExists(t, filepath.Join(archiveDir, "000-large.md"))

	splitTasks, err := filepath.Glob(filepath.Join(splitDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(splitTasks) != 3 {
		t.Fatalf("expected 3 split tasks, got %d: %v", len(splitTasks), splitTasks)
	}
}

func TestWorkflowSplitTaskExecutesOneTaskOnly(t *testing.T) {
	chdirTemp(t)
	restore := stubRunAgent(t, []byte("split ok\n"), nil)
	defer restore()

	writeTask(t, filepath.Join(splitDir, "001-split.md"), "# Split 1\nDo one thing.")
	writeTask(t, filepath.Join(splitDir, "002-split.md"), "# Split 2\nDo another thing.")

	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Queue: split") {
		t.Fatalf("expected split queue output, got %s", out.String())
	}
	assertExists(t, filepath.Join(doneDir, "001-split.md"))
	assertExists(t, filepath.Join(splitDir, "002-split.md"))
	assertMissing(t, filepath.Join(doneDir, "002-split.md"))
}

func stubRunAgent(t *testing.T, output []byte, err error) func() {
	t.Helper()

	previous := runAgent
	runAgent = func(taskPath string, maxSteps int) ([]byte, error) {
		if filepath.Dir(taskPath) != processDir {
			t.Fatalf("agent task path = %q, want process dir", taskPath)
		}
		return output, err
	}

	return func() {
		runAgent = previous
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, stat err: %v", path, err)
	}
}

func assertEmptyDir(t *testing.T, path string) {
	t.Helper()

	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read dir %s: %v", path, err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected %s to be empty, got %d entries", path, len(entries))
	}
}
