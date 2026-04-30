package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTaskValidTaskReturnsNoErrors(t *testing.T) {
	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		AllowedFiles:   []string{"internal/tasks/validator.go"},
		Validation:     []string{"go test ./internal/tasks"},
	}

	errs := ValidateTask(task)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
}

func TestValidateTaskMissingTaskIDReturnsError(t *testing.T) {
	task := Task{
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		AllowedFiles:   []string{"internal/tasks/validator.go"},
		Validation:     []string{"go test ./internal/tasks"},
	}

	assertHasValidationError(t, ValidateTask(task), "task_id")
}

func TestValidateTaskEmptyAllowedFilesReturnsError(t *testing.T) {
	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		Validation:     []string{"go test ./internal/tasks"},
	}

	assertHasValidationError(t, ValidateTask(task), "allowed_files")
}

func TestValidateTaskAbsoluteAllowedFilePathReturnsError(t *testing.T) {
	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		AllowedFiles:   []string{"/tmp/file.go"},
		Validation:     []string{"go test ./internal/tasks"},
	}

	assertHasValidationError(t, ValidateTask(task), "allowed_files")
}

func TestValidateTaskParentDirectoryTraversalReturnsError(t *testing.T) {
	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		AllowedFiles:   []string{"../secret.go"},
		Validation:     []string{"go test ./internal/tasks"},
	}

	assertHasValidationError(t, ValidateTask(task), "allowed_files")
}

func TestValidateTaskInvalidExecutionGroupReturnsError(t *testing.T) {
	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "weird",
		AllowedFiles:   []string{"internal/tasks/validator.go"},
		Validation:     []string{"go test ./internal/tasks"},
	}

	assertHasValidationError(t, ValidateTask(task), "execution_group")
}

func TestValidateTaskSelfDependencyReturnsError(t *testing.T) {
	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		DependsOn:      []string{"task-1"},
		AllowedFiles:   []string{"internal/tasks/validator.go"},
		Validation:     []string{"go test ./internal/tasks"},
	}

	assertHasValidationError(t, ValidateTask(task), "depends_on")
}

func TestValidateTaskMissingPackScriptReturnsError(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "task-scripts"))
	mustMkdir(t, filepath.Join(root, "snippets"))
	mustWriteFile(t, filepath.Join(root, "snippets", "ok.go"), "package snippets\n")

	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		AllowedFiles:   []string{"internal/tasks/validator.go"},
		Validation:     []string{"go test ./internal/tasks"},
		Scripts:        []string{"task-scripts/missing.md"},
		Snippets:       []string{"snippets/ok.go"},
		PackContext: &TaskPackContext{
			Root:           root,
			TaskScriptsDir: "task-scripts",
			SnippetsDir:    "snippets",
		},
	}

	assertHasValidationError(t, ValidateTask(task), "scripts")
}

func TestValidateTaskMissingPackSnippetReturnsError(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "task-scripts"))
	mustMkdir(t, filepath.Join(root, "snippets"))
	mustWriteFile(t, filepath.Join(root, "task-scripts", "ok.md"), "# ok\n")

	task := Task{
		ID:             "task-1",
		Status:         StatusInbox,
		Branch:         "ptolemy/task-1",
		ExecutionGroup: "sequential",
		AllowedFiles:   []string{"internal/tasks/validator.go"},
		Validation:     []string{"go test ./internal/tasks"},
		Scripts:        []string{"task-scripts/ok.md"},
		Snippets:       []string{"snippets/missing.go"},
		PackContext: &TaskPackContext{
			Root:           root,
			TaskScriptsDir: "task-scripts",
			SnippetsDir:    "snippets",
		},
	}

	assertHasValidationError(t, ValidateTask(task), "snippets")
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertHasValidationError(t *testing.T, errs []ValidationError, field string) {
	t.Helper()

	for _, err := range errs {
		if err.Field == field {
			return
		}
	}

	t.Fatalf("expected validation error for %s, got %+v", field, errs)
}
