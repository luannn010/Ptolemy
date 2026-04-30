package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ValidationError struct {
	TaskID string
	Field  string
	Reason string
}

func ValidateTask(task Task) []ValidationError {
	errs := make([]ValidationError, 0)

	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		taskID = "<unknown>"
	}

	if strings.TrimSpace(task.ID) == "" {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "task_id",
			Reason: "is required",
		})
	}

	if strings.TrimSpace(task.Branch) == "" {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "branch",
			Reason: "is required",
		})
	}

	if !isAllowedStatus(task.Status) {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "status",
			Reason: "must be one of inbox, running, completed, failed, or blocked",
		})
	}

	if !isAllowedExecutionGroup(task.ExecutionGroup) {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "execution_group",
			Reason: "must be one of sequential, parallel, or final",
		})
	}

	if len(task.AllowedFiles) == 0 {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "allowed_files",
			Reason: "must contain at least one path",
		})
	}

	for _, allowed := range task.AllowedFiles {
		errs = append(errs, validateAllowedFile(taskID, allowed)...)
	}

	if len(task.Validation) == 0 {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "validation",
			Reason: "must contain at least one command",
		})
	}

	for _, dep := range task.DependsOn {
		if strings.TrimSpace(dep) == strings.TrimSpace(task.ID) && strings.TrimSpace(dep) != "" {
			errs = append(errs, ValidationError{
				TaskID: taskID,
				Field:  "depends_on",
				Reason: "must not contain the task's own task_id",
			})
			break
		}
	}

	if task.PackContext != nil {
		for _, script := range task.Scripts {
			errs = append(errs, validatePackReference(taskID, "scripts", script, task.PackContext.Root, task.PackContext.TaskScriptsDir)...)
		}
		for _, snippet := range task.Snippets {
			errs = append(errs, validatePackReference(taskID, "snippets", snippet, task.PackContext.Root, task.PackContext.SnippetsDir)...)
		}
	}

	return errs
}

func ValidateTasks(tasks []Task) []ValidationError {
	all := make([]ValidationError, 0)
	for _, task := range tasks {
		all = append(all, ValidateTask(task)...)
	}
	return all
}

func isAllowedStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case StatusInbox, StatusRunning, StatusCompleted, StatusFailed, StatusBlocked:
		return true
	default:
		return false
	}
}

func isAllowedExecutionGroup(group string) bool {
	switch strings.TrimSpace(group) {
	case "sequential", "parallel", "final":
		return true
	default:
		return false
	}
}

func validateAllowedFile(taskID string, allowed string) []ValidationError {
	errs := make([]ValidationError, 0)
	trimmed := strings.TrimSpace(allowed)

	if trimmed == "" {
		return []ValidationError{{
			TaskID: taskID,
			Field:  "allowed_files",
			Reason: "must not contain empty paths",
		}}
	}

	if filepath.IsAbs(trimmed) {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "allowed_files",
			Reason: "must not contain absolute paths",
		})
	}

	cleaned := filepath.Clean(trimmed)
	parts := strings.Split(filepath.ToSlash(cleaned), "/")
	for _, part := range parts {
		if part == ".." {
			errs = append(errs, ValidationError{
				TaskID: taskID,
				Field:  "allowed_files",
				Reason: "must not contain parent directory traversal",
			})
			break
		}
	}

	if strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, "\\") || filepath.Base(cleaned) == "." {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  "allowed_files",
			Reason: "must not contain directory paths",
		})
	}

	return errs
}

func validatePackReference(taskID string, field string, value string, packRoot string, expectedDir string) []ValidationError {
	errs := make([]ValidationError, 0)
	trimmed := strings.TrimSpace(value)

	if trimmed == "" {
		return []ValidationError{{
			TaskID: taskID,
			Field:  field,
			Reason: "must not contain empty paths",
		}}
	}

	if filepath.IsAbs(trimmed) {
		return []ValidationError{{
			TaskID: taskID,
			Field:  field,
			Reason: "must not contain absolute paths",
		}}
	}

	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return []ValidationError{{
				TaskID: taskID,
				Field:  field,
				Reason: "must not contain parent directory traversal",
			}}
		}
	}

	expectedPrefix := normalizeRelativeDir(expectedDir) + "/"
	if cleaned != normalizeRelativeDir(expectedDir) && !strings.HasPrefix(cleaned, expectedPrefix) {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  field,
			Reason: fmt.Sprintf("must stay within %s", normalizeRelativeDir(expectedDir)),
		})
		return errs
	}

	resolvedPath := filepath.Join(packRoot, filepath.FromSlash(cleaned))
	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			errs = append(errs, ValidationError{
				TaskID: taskID,
				Field:  field,
				Reason: fmt.Sprintf("referenced file does not exist: %s", cleaned),
			})
			return errs
		}
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  field,
			Reason: err.Error(),
		})
		return errs
	}
	if info.IsDir() {
		errs = append(errs, ValidationError{
			TaskID: taskID,
			Field:  field,
			Reason: fmt.Sprintf("expected file, found directory: %s", cleaned),
		})
	}

	return errs
}
