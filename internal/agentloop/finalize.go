package agentloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/tasks"
)

type Finalizer struct {
	artifactsDir string
}

func NewFinalizer(artifactsDir string) *Finalizer {
	return &Finalizer{artifactsDir: artifactsDir}
}

func (f *Finalizer) Finalize(ctx context.Context, run Run, task tasks.Task) (string, error) {
	workspace := run.WorktreePath
	if strings.TrimSpace(workspace) == "" {
		workspace = run.Workspace
	}
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("workspace is required")
	}

	validationResult := tasks.NewRunner(workspace).RunValidation(ctx, task)
	if !validationResult.Success {
		return f.writeReport(run.ID, "validation-failed.json", validationResult), fmt.Errorf("task validation failed")
	}

	git := gitops.New(workspace)
	changedFilesResult := git.ChangedFiles(ctx)
	if !changedFilesResult.Success {
		return "", fmt.Errorf("list changed files: %s", strings.TrimSpace(changedFilesResult.Output))
	}

	changedFiles := filterAllowedFiles(strings.Split(changedFilesResult.Output, "\n"), task.AllowedFiles)
	if len(changedFiles) == 0 {
		return "", fmt.Errorf("no changed task files to stage")
	}

	stageResult := git.StageFiles(ctx, changedFiles)
	if !stageResult.Success {
		return "", fmt.Errorf("stage files: %s", strings.TrimSpace(stageResult.Output))
	}

	commitMessage := fmt.Sprintf("feat(agentloop): complete %s", task.ID)
	commitResult := git.CommitStagedConventional(ctx, commitMessage)
	if !commitResult.Success {
		return "", fmt.Errorf("commit staged files: %s", strings.TrimSpace(commitResult.Output))
	}

	commitSHA := strings.TrimSpace(git.CurrentCommitSHA(ctx).Output)
	_, kbErr := navigator.UpdateKnowledgeBase(workspace, changedFiles, "", []string{task.ID}, commitSHA)
	pushResult := git.Push(ctx, "origin", task.Branch)
	prResult := git.CreatePullRequest(ctx, "main", task.Branch, commitMessage, "")

	report := fmt.Sprintf(
		"{\"status\":\"completed\",\"task_id\":%q,\"commit_sha\":%q,\"push_success\":%t,\"pr_success\":%t,\"kb_error\":%q}",
		task.ID,
		commitSHA,
		pushResult.Success,
		prResult.Success,
		errorString(kbErr),
	)
	return f.writeReport(run.ID, "final-report.json", report), nil
}

func filterAllowedFiles(changedFiles []string, allowedFiles []string) []string {
	var filtered []string
	for _, changed := range changedFiles {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(changed)))
		if clean == "." || clean == "" {
			continue
		}
		if isAllowedPath(allowedFiles, clean) {
			filtered = append(filtered, clean)
		}
	}
	return filtered
}

func (f *Finalizer) writeReport(runID string, name string, payload any) string {
	if strings.TrimSpace(f.artifactsDir) == "" {
		return ""
	}
	dir := filepath.Join(f.artifactsDir, runID)
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, name)
	_ = os.WriteFile(path, []byte(fmt.Sprint(payload)), 0o644)
	return path
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
