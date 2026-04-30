package tasks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/luannn010/ptolemy/internal/gitops"
)

func runTaskPackList(ctx context.Context, pack TaskPack, workspace string) SchedulerResult {
	result := SchedulerResult{
		PlannedTaskIDs:   []string{},
		CompletedTaskIDs: []string{},
		ValidationErrors: []ValidationError{},
		TaskLogPaths:     map[string]string{},
		PreparedBranches: map[string]string{},
	}

	result.ValidationErrors = ValidateTasks(pack.Tasks)
	if len(result.ValidationErrors) > 0 {
		return result
	}

	plan, err := BuildExecutionPlan(pack.Tasks, map[string]bool{})
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			TaskID: "<plan>",
			Field:  "depends_on",
			Reason: err.Error(),
		})
		return result
	}

	artifactDir, err := ensurePackArtifactDir(pack)
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			TaskID: pack.Manifest.PackID,
			Field:  "artifacts",
			Reason: err.Error(),
		})
		return result
	}

	if pack.Manifest.Rules.RequireBranch {
		prepared, branchErr := preparePackBranches(ctx, workspace, artifactDir, plan)
		for taskID, branch := range prepared {
			result.PreparedBranches[taskID] = branch
		}
		if branchErr != nil {
			result.FailedTaskID = "<branch-preparation>"
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				TaskID: "<branch-preparation>",
				Field:  "branch",
				Reason: branchErr.Error(),
			})
			result.SummaryPath, _ = writePackSummary(pack, artifactDir, result)
			return result
		}
	}

	runner := NewRunner(workspace)
	for _, step := range plan.Steps {
		task := step.Task
		result.PlannedTaskIDs = append(result.PlannedTaskIDs, task.ID)

		if err := UpdateTaskStatusFile(task.Path, StatusRunning); err != nil {
			result.FailedTaskID = task.ID
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				TaskID: task.ID,
				Field:  "status",
				Reason: err.Error(),
			})
			result.SummaryPath, _ = writePackSummary(pack, artifactDir, result)
			return result
		}

		runResult := runner.RunValidation(ctx, task)
		logPath, logErr := writePackTaskLog(artifactDir, task, runResult)
		if logErr == nil {
			result.TaskLogPaths[task.ID] = logPath
		}
		if logErr != nil {
			result.FailedTaskID = task.ID
			_ = UpdateTaskStatusFile(task.Path, StatusFailed)
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				TaskID: task.ID,
				Field:  "artifacts",
				Reason: logErr.Error(),
			})
			result.SummaryPath, _ = writePackSummary(pack, artifactDir, result)
			return result
		}

		if runResult.Success {
			if err := UpdateTaskStatusFile(task.Path, StatusCompleted); err != nil {
				result.FailedTaskID = task.ID
				result.ValidationErrors = append(result.ValidationErrors, ValidationError{
					TaskID: task.ID,
					Field:  "status",
					Reason: err.Error(),
				})
				result.SummaryPath, _ = writePackSummary(pack, artifactDir, result)
				return result
			}
			result.CompletedTaskIDs = append(result.CompletedTaskIDs, task.ID)
			continue
		}

		_ = UpdateTaskStatusFile(task.Path, StatusFailed)
		result.FailedTaskID = task.ID
		result.IssueDraftPath, _ = writePackIssueDraft(pack, artifactDir, task, logPath)
		result.SummaryPath, _ = writePackSummary(pack, artifactDir, result)
		return result
	}

	result.PRDraftPath, _ = writePackPRDraft(pack, artifactDir, result)
	result.SummaryPath, _ = writePackSummary(pack, artifactDir, result)
	return result
}

func ensurePackArtifactDir(pack TaskPack) (string, error) {
	artifactDir := filepath.Join(pack.Root, ".state", "task-packs", sanitizeArtifactName(pack.Manifest.PackID))
	for _, dir := range []string{
		artifactDir,
		filepath.Join(artifactDir, "tasks"),
		filepath.Join(artifactDir, "branches"),
		filepath.Join(artifactDir, "github"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return artifactDir, nil
}

func preparePackBranches(ctx context.Context, workspace string, artifactDir string, plan ExecutionPlan) (map[string]string, error) {
	resolvedWorkspace, err := resolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	git := gitops.New(resolvedWorkspace)
	prepared := map[string]string{}
	seen := map[string]bool{}

	for _, step := range plan.Steps {
		task := step.Task
		if seen[task.Branch] {
			prepared[task.ID] = task.Branch
			continue
		}

		branchResult := git.EnsureBranch(ctx, task.Branch)
		if _, writeErr := writePackBranchLog(artifactDir, task, branchResult); writeErr != nil {
			return prepared, writeErr
		}
		if !branchResult.Success {
			return prepared, fmt.Errorf("prepare branch %s: %s", task.Branch, strings.TrimSpace(branchResult.Output))
		}

		seen[task.Branch] = true
		prepared[task.ID] = task.Branch
	}

	return prepared, nil
}

func resolveWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) != "" {
		return filepath.Abs(workspace)
	}
	return os.Getwd()
}

func writePackTaskLog(artifactDir string, task Task, runResult TaskRunResult) (string, error) {
	path := filepath.Join(artifactDir, "tasks", sanitizeArtifactName(task.ID)+".log")
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("task_id: %s\n", task.ID))
	builder.WriteString(fmt.Sprintf("branch: %s\n", task.Branch))
	builder.WriteString(fmt.Sprintf("success: %t\n", runResult.Success))

	for i, commandResult := range runResult.Results {
		builder.WriteString(fmt.Sprintf("\n## command %d\n", i+1))
		builder.WriteString(fmt.Sprintf("command: %s\n", commandResult.Command))
		builder.WriteString(fmt.Sprintf("exit_code: %d\n", commandResult.ExitCode))
		builder.WriteString("stdout:\n")
		builder.WriteString(commandResult.Stdout)
		if !strings.HasSuffix(commandResult.Stdout, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("stderr:\n")
		builder.WriteString(commandResult.Stderr)
		if !strings.HasSuffix(commandResult.Stderr, "\n") {
			builder.WriteString("\n")
		}
	}

	return path, os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writePackBranchLog(artifactDir string, task Task, result gitops.Result) (string, error) {
	path := filepath.Join(artifactDir, "branches", sanitizeArtifactName(task.ID)+".log")
	content := fmt.Sprintf(
		"task_id: %s\nbranch: %s\nsuccess: %t\ncommand: %s\nexit_code: %d\noutput:\n%s",
		task.ID,
		task.Branch,
		result.Success,
		result.Command,
		result.ExitCode,
		result.Output,
	)
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func writePackIssueDraft(pack TaskPack, artifactDir string, task Task, logPath string) (string, error) {
	path := filepath.Join(artifactDir, "github", "issue-draft-"+sanitizeArtifactName(task.ID)+".md")
	content := fmt.Sprintf(`# Issue Draft: %s

## Summary

Task pack %q failed while running task %q.

## Details

- Pack: %q
- Task: %q
- Branch: %q
- Task file: %q
- Log: %q

## Suggested next step

Review the task log and open a GitHub issue with the failure details above.
`, task.ID, pack.Manifest.Name, task.ID, pack.Manifest.PackID, task.ID, task.Branch, task.Path, logPath)
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func writePackPRDraft(pack TaskPack, artifactDir string, result SchedulerResult) (string, error) {
	path := filepath.Join(artifactDir, "github", "pull-request-draft.md")
	branches := uniqueSortedBranches(result.PreparedBranches)
	content := fmt.Sprintf(`# Pull Request Draft: %s

## Summary

Task pack %q completed successfully.

## Target

- Base: %q

## Completed tasks
%s

## Prepared branches
%s
`, pack.Manifest.Name, pack.Manifest.PackID, "main", markdownList(result.CompletedTaskIDs), markdownList(branches))
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func writePackSummary(pack TaskPack, artifactDir string, result SchedulerResult) (string, error) {
	path := filepath.Join(artifactDir, "summary.txt")
	content := fmt.Sprintf(
		"pack_id: %s\nplanned: %s\ncompleted: %s\nfailed: %s\nissue_draft: %s\npr_draft: %s\n",
		pack.Manifest.PackID,
		strings.Join(result.PlannedTaskIDs, ","),
		strings.Join(result.CompletedTaskIDs, ","),
		result.FailedTaskID,
		result.IssueDraftPath,
		result.PRDraftPath,
	)
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func sanitizeArtifactName(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(strings.TrimSpace(value))
}

func uniqueSortedBranches(prepared map[string]string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(prepared))
	for _, branch := range prepared {
		if seen[branch] || strings.TrimSpace(branch) == "" {
			continue
		}
		seen[branch] = true
		out = append(out, branch)
	}
	slices.Sort(out)
	return out
}

func markdownList(items []string) string {
	if len(items) == 0 {
		return "- none"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, "- `"+item+"`")
	}
	return strings.Join(lines, "\n")
}
