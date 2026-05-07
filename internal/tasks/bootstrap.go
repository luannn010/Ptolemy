package tasks

import (
	"fmt"
	"os"
	"path/filepath"
)

type BootstrapResult struct {
	Workspace    string
	CreatedFiles []string
	CreatedDirs  []string
}

func BootstrapWorkspace(workspace string) (BootstrapResult, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("resolve workspace: %w", err)
	}

	info, err := os.Stat(root)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return BootstrapResult{}, fmt.Errorf("workspace is not a directory: %s", workspace)
	}

	result := BootstrapResult{Workspace: root}

	for _, rel := range []string{
		"docs/tasks/inbox",
		"docs/tasks/active",
		"docs/tasks/process",
		"docs/tasks/split",
		"docs/tasks/done",
		"docs/tasks/failed",
		"docs/tasks/archive",
		"docs/tasks/templates",
	} {
		created, err := ensureBootstrapDir(root, rel)
		if err != nil {
			return BootstrapResult{}, err
		}
		if created {
			result.CreatedDirs = append(result.CreatedDirs, rel)
		}
	}

	for rel, content := range map[string]string{
		"WORKFLOWS.md": defaultWorkflowIndex(),
		"docs/tasks/templates/task-file-template.md":  defaultTaskTemplate(),
		"docs/tasks/templates/split-task-template.md": defaultSplitTaskTemplate(),
	} {
		created, err := ensureBootstrapFile(root, rel, content)
		if err != nil {
			return BootstrapResult{}, err
		}
		if created {
			result.CreatedFiles = append(result.CreatedFiles, rel)
		}
	}

	return result, nil
}

func ensureBootstrapDir(root string, rel string) (bool, error) {
	target := filepath.Join(root, filepath.FromSlash(rel))
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return false, nil
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return false, fmt.Errorf("create directory %s: %w", rel, err)
	}
	return true, nil
}

func ensureBootstrapFile(root string, rel string, content string) (bool, error) {
	target := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(target); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, fmt.Errorf("create parent directory for %s: %w", rel, err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write bootstrap file %s: %w", rel, err)
	}
	return true, nil
}

func defaultWorkflowIndex() string {
	return "# Ptolemy Workflows\n\n" +
		"This file is the workflow entry point for repositories managed through Ptolemy.\n\n" +
		"Start with the smallest safe workflow for the task:\n\n" +
		"- Read only the files you need.\n" +
		"- Search before full-file reads.\n" +
		"- Keep edits small and reversible.\n" +
		"- Validate the result before commit.\n" +
		"- Use task metadata to keep file scope explicit.\n"
}

func defaultTaskTemplate() string {
	return "---\n" +
		"priority: normal\n" +
		"task_id: example-task-id\n" +
		"parent_task: null\n" +
		"owner: unassigned\n" +
		"status: inbox\n" +
		"branch: ptolemy/normal-example-task-id\n" +
		"allowed_files:\n" +
		"  - README.md\n" +
		"validation:\n" +
		"  - printf 'add validation command here\\n'\n" +
		"created_by: codex\n" +
		"---\n\n" +
		"# Goal\n\n" +
		"Describe the task outcome in one short paragraph.\n\n" +
		"# Scope\n\n" +
		"List the exact files this task may modify.\n\n" +
		"# Validation\n\n" +
		"List the commands that must pass before the task is complete.\n"
}

func defaultSplitTaskTemplate() string {
	return "---\n" +
		"priority: normal\n" +
		"task_id: example-task-id-part-1\n" +
		"parent_task: example-task-id\n" +
		"owner: unassigned\n" +
		"status: split\n" +
		"branch: ptolemy/normal-example-task-id-part-1\n" +
		"allowed_files:\n" +
		"  - README.md\n" +
		"validation:\n" +
		"  - printf 'add validation command here\\n'\n" +
		"created_by: codex\n" +
		"---\n\n" +
		"# Parent Task\n\n" +
		"Reference the parent task and explain the narrowed scope.\n\n" +
		"# Goal\n\n" +
		"Describe the split task outcome in one short paragraph.\n\n" +
		"# Validation\n\n" +
		"List the commands that must pass before the split task is complete.\n"
}
