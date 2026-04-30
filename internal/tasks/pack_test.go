package tasks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTaskPackRejectsMissingManifest(t *testing.T) {
	root := createPackSkeleton(t)
	if err := os.Remove(filepath.Join(root, "PACK_MANIFEST.yaml")); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTaskPack(root)
	if err == nil || !strings.Contains(err.Error(), "PACK_MANIFEST.yaml") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}
}

func TestLoadTaskPackRejectsMissingTaskPlan(t *testing.T) {
	root := createPackSkeleton(t)
	if err := os.Remove(filepath.Join(root, "TASK_PLAN.md")); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTaskPack(root)
	if err == nil || !strings.Contains(err.Error(), "TASK_PLAN.md") {
		t.Fatalf("expected missing task plan error, got %v", err)
	}
}

func TestLoadTaskPackRejectsMissingRequiredFolder(t *testing.T) {
	root := createPackSkeleton(t)
	if err := os.RemoveAll(filepath.Join(root, "snippets")); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTaskPack(root)
	if err == nil || !strings.Contains(err.Error(), "snippets") {
		t.Fatalf("expected missing folder error, got %v", err)
	}
}

func TestLoadTaskPackRejectsUnsupportedExecutionMode(t *testing.T) {
	root := createPackSkeleton(t)
	mustWriteTaskFile(t, filepath.Join(root, "PACK_MANIFEST.yaml"), packManifest("parallel_first"))

	_, err := LoadTaskPack(root)
	if err == nil || !strings.Contains(err.Error(), "unsupported execution_mode") {
		t.Fatalf("expected execution mode error, got %v", err)
	}
}

func TestLoadTaskPackRejectsMissingInboxFolderSetting(t *testing.T) {
	root := createPackSkeleton(t)
	mustWriteTaskFile(t, filepath.Join(root, "PACK_MANIFEST.yaml"), strings.Replace(packManifest("sequential_first"), "  inbox: inbox\n", "", 1))

	_, err := LoadTaskPack(root)
	if err == nil || !strings.Contains(err.Error(), "folders.inbox") {
		t.Fatalf("expected folders.inbox error, got %v", err)
	}
}

func TestBuildPackPlanPreviewReturnsOrderedTaskIDs(t *testing.T) {
	root := createPackSkeleton(t)
	writePackTask(t, filepath.Join(root, "inbox"), "b.md", "task-b", "inbox", "ptolemy/task-b", "parallel", []string{"task-a"}, []string{"printf b"}, nil, nil)
	writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"}, nil, nil)

	ids, validationErrs, err := BuildPackPlanPreview(root)
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

func TestBuildPackPlanPreviewReturnsValidationErrorsForMissingAssets(t *testing.T) {
	root := createPackSkeleton(t)
	writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"}, []string{"task-scripts/missing.md"}, []string{"snippets/example.go"})
	mustWriteTaskFile(t, filepath.Join(root, "snippets", "example.go"), "package snippets\n")

	_, validationErrs, err := BuildPackPlanPreview(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validationErrs) == 0 {
		t.Fatal("expected validation errors")
	}
	assertHasValidationError(t, validationErrs, "scripts")
}

func TestRunTaskPackRunsTasksInDependencyOrder(t *testing.T) {
	root := createPackSkeleton(t)
	first := writePackTask(t, filepath.Join(root, "inbox"), "b.md", "task-b", "inbox", "ptolemy/task-b", "sequential", []string{"task-a"}, []string{"printf b"}, nil, nil)
	second := writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"}, nil, nil)

	result := RunTaskPack(context.Background(), root, "")
	if result.FailedTaskID != "" || len(result.ValidationErrors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.PlannedTaskIDs) != 2 || result.PlannedTaskIDs[0] != "task-a" || result.PlannedTaskIDs[1] != "task-b" {
		t.Fatalf("unexpected plan: %+v", result.PlannedTaskIDs)
	}

	aData, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	bData, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(aData), "status: completed") || !strings.Contains(string(bData), "status: completed") {
		t.Fatalf("expected completed statuses, got\nA:%s\nB:%s", string(aData), string(bData))
	}
}

func createPackSkeleton(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "scripts"))
	mustMkdir(t, filepath.Join(root, "task-scripts"))
	mustMkdir(t, filepath.Join(root, "snippets"))
	mustMkdir(t, filepath.Join(root, "inbox"))

	mustWriteTaskFile(t, filepath.Join(root, "TASK_PLAN.md"), "# Task Plan\n")
	mustWriteTaskFile(t, filepath.Join(root, "README.md"), "# Pack\n")
	mustWriteTaskFile(t, filepath.Join(root, "PACK_MANIFEST.yaml"), packManifest("sequential_first"))

	return root
}

func packManifest(mode string) string {
	return "pack_id: sample-pack\n" +
		"name: Sample Pack\n" +
		"version: 1\n" +
		"created_by: test\n" +
		"entrypoint: TASK_PLAN.md\n" +
		"folders:\n" +
		"  inbox: inbox\n" +
		"  scripts: scripts\n" +
		"  task_scripts: task-scripts\n" +
		"  snippets: snippets\n" +
		"execution_mode: " + mode + "\n" +
		"validation:\n" +
		"  - go test ./internal/tasks\n" +
		"rules:\n" +
		"  max_allowed_files: 8\n" +
		"  require_validation: true\n" +
		"  require_branch: true\n" +
		"  stop_on_failure: true\n"
}

func writePackTask(t *testing.T, dir string, name string, id string, status string, branch string, group string, deps []string, validation []string, scripts []string, snippets []string) string {
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

	if len(scripts) > 0 {
		content += "scripts:\n"
		for _, script := range scripts {
			content += "  - " + script + "\n"
		}
	}

	if len(snippets) > 0 {
		content += "snippets:\n"
		for _, snippet := range snippets {
			content += "  - " + snippet + "\n"
		}
	}

	content += "validation:\n"
	for _, cmd := range validation {
		content += "  - " + cmd + "\n"
	}
	content += "---\nbody\n"

	path := filepath.Join(dir, name)
	mustWriteTaskFile(t, path, content)
	return path
}

func mustWriteTaskFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
