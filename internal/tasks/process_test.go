package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEstimateTokensUsesFourCharHeuristic(t *testing.T) {
	if got := EstimateTokens(strings.Repeat("a", 9)); got != 3 {
		t.Fatalf("EstimateTokens() = %d, want 3", got)
	}
}

func TestBuildProcessManifestSplitsLargeTaskIntoChildTasks(t *testing.T) {
	task := Task{
		ID:           "feature-pack",
		Path:         "docs/tasks/inbox/feature-pack.md",
		Branch:       "ptolemy/feature-pack",
		AllowedFiles: []string{"internal/service/core.go", "internal/service/core_test.go", "docs/feature.md"},
		Validation:   []string{"go test ./internal/service"},
		Body: `# Feature Pack

This is a full implementation with multiple phases.

## Inspect
Review internal/service/core.go and document the current behavior.

## Implement
Update internal/service/core.go with the core change.

## Validate
Run go test ./internal/service and summarize the result.
`,
	}

	manifest := BuildProcessManifest(task)
	if manifest.PackID != "feature-pack" {
		t.Fatalf("unexpected pack id: %s", manifest.PackID)
	}
	if len(manifest.ChildTasks) != 3 {
		t.Fatalf("expected 3 child tasks, got %d", len(manifest.ChildTasks))
	}
	if manifest.ChildTasks[0].Phase != "inspect" {
		t.Fatalf("first child phase = %q, want inspect", manifest.ChildTasks[0].Phase)
	}
	if manifest.ChildTasks[1].Phase != "edit" {
		t.Fatalf("second child phase = %q, want edit", manifest.ChildTasks[1].Phase)
	}
	if manifest.ChildTasks[2].Phase != "validate" {
		t.Fatalf("third child phase = %q, want validate", manifest.ChildTasks[2].Phase)
	}
	if len(manifest.ChildTasks[2].ValidationCommands) != 1 {
		t.Fatalf("expected validation commands on validate child, got %+v", manifest.ChildTasks[2].ValidationCommands)
	}
}

func TestWriteAndLoadProcessManifestAndSummary(t *testing.T) {
	workspace := t.TempDir()
	task := Task{
		ID:           "feature-pack",
		Path:         "docs/tasks/inbox/feature-pack.md",
		Branch:       "ptolemy/feature-pack",
		AllowedFiles: []string{"internal/service/core.go"},
		Validation:   []string{"go test ./internal/service"},
		Body:         "# Feature Pack\n\nBuild a full implementation with multiple phases.\n",
	}

	manifest := BuildProcessManifest(task)
	paths, err := EnsureProcessFiles(workspace, manifest)
	if err != nil {
		t.Fatalf("EnsureProcessFiles() error = %v", err)
	}

	manifest.Status = ProcessStatusRunning
	manifest.CurrentChildTaskID = manifest.ChildTasks[0].ID
	if err := WriteProcessManifest(paths.ManifestPath, manifest); err != nil {
		t.Fatalf("WriteProcessManifest() error = %v", err)
	}
	if err := WriteProcessTodo(paths.TodoPath, manifest); err != nil {
		t.Fatalf("WriteProcessTodo() error = %v", err)
	}

	loaded, err := LoadProcessManifest(paths)
	if err != nil {
		t.Fatalf("LoadProcessManifest() error = %v", err)
	}
	if loaded.CurrentChildTaskID != manifest.CurrentChildTaskID {
		t.Fatalf("loaded current child = %q, want %q", loaded.CurrentChildTaskID, manifest.CurrentChildTaskID)
	}

	summaryPath, err := WriteProcessSummary(paths, ProcessSummary{
		ChildTaskID:         loaded.ChildTasks[0].ID,
		Title:               loaded.ChildTasks[0].Title,
		Status:              ProcessStatusDone,
		FilesInspected:      []string{"internal/service/core.go"},
		FilesChanged:        []string{"internal/service/core.go"},
		CommandsRun:         []string{"go test ./internal/service"},
		ValidationResult:    "success",
		ImportantDecisions:  []string{"kept the change scoped"},
		NextRecommendedStep: "continue to the next child task",
	})
	if err != nil {
		t.Fatalf("WriteProcessSummary() error = %v", err)
	}
	if !strings.Contains(summaryPath, ".ptolemy/tasks/process/feature-pack/state/") {
		t.Fatalf("unexpected summary path: %s", summaryPath)
	}

	todoData, err := os.ReadFile(paths.TodoPath)
	if err != nil {
		t.Fatalf("read todo: %v", err)
	}
	if !strings.Contains(string(todoData), manifest.CurrentChildTaskID) {
		t.Fatalf("todo file missing current child task: %s", string(todoData))
	}

	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(summaryPath))); err != nil {
		t.Fatalf("summary file missing: %v", err)
	}
}
