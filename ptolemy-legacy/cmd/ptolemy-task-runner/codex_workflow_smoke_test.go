package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexWorkflowSmokeProcessesActiveTask(t *testing.T) {
	chdirTemp(t)
	restore := stubRunAgent(t, []byte("codex smoke ok\n"), nil)
	defer restore()

	writeTask(t, filepath.Join(activeDir, "000-codex-smoke.md"), "# Codex Smoke\nValidate the active task workflow.")

	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Selected task: docs/tasks/process/000-codex-smoke.md",
		"Queue: active",
		"Classification: small",
		"Max steps: 4",
		"Result: completed",
		"Log: .state/task-runner/000-codex-smoke-output.txt",
		"Archive: docs/tasks/archive/000-codex-smoke.md",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("run() output missing %q:\n%s", want, output)
		}
	}

	assertEmptyDir(t, activeDir)
	assertEmptyDir(t, processDir)
	assertExists(t, filepath.Join(doneDir, "000-codex-smoke.md"))
	assertExists(t, filepath.Join(archiveDir, "000-codex-smoke.md"))
	assertExists(t, filepath.Join(taskRunnerStateDir, "000-codex-smoke-output.txt"))
}
