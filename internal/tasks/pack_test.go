package tasks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	repo, _ := setupPackRepoWithPublish(t)
	first := writePackTask(t, filepath.Join(root, "inbox"), "b.md", "task-b", "inbox", "ptolemy/task-b", "sequential", []string{"task-a"}, []string{"printf b"}, nil, nil)
	second := writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"}, nil, nil)

	result := RunTaskPack(context.Background(), root, repo)
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

func TestRunTaskPackWritesArtifactsAndPullRequestDraft(t *testing.T) {
	root := createPackSkeleton(t)
	repo, ghLogPath := setupPackRepoWithPublish(t)
	createFeatureBranchCommit(t, repo, "ptolemy/task-a", "task-a.txt", "A\n")
	createFeatureBranchCommit(t, repo, "ptolemy/task-b", "task-b.txt", "B\n")
	writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"}, nil, nil)
	writePackTask(t, filepath.Join(root, "inbox"), "b.md", "task-b", "inbox", "ptolemy/task-b", "sequential", []string{"task-a"}, []string{"printf b"}, nil, nil)

	result := RunTaskPack(context.Background(), root, repo)
	if result.FailedTaskID != "" || len(result.ValidationErrors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.PRDraftPath == "" {
		t.Fatalf("expected PR draft path, got %+v", result)
	}
	if result.PullRequestURL != "https://example.com/pr/123" {
		t.Fatalf("expected PR URL, got %+v", result)
	}
	if result.IntegrationBranch == "" || result.PushLogPath == "" || result.PRCreateLogPath == "" {
		t.Fatalf("expected publish artifacts, got %+v", result)
	}
	if result.SummaryPath == "" {
		t.Fatalf("expected summary path, got %+v", result)
	}
	if len(result.TaskLogPaths) != 2 {
		t.Fatalf("expected task logs, got %+v", result.TaskLogPaths)
	}

	logData, err := os.ReadFile(result.TaskLogPaths["task-a"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "command: printf a") {
		t.Fatalf("expected task log content, got %s", string(logData))
	}

	prData, err := os.ReadFile(result.PRDraftPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prData), "Base: \"main\"") || !strings.Contains(string(prData), result.IntegrationBranch) {
		t.Fatalf("unexpected PR draft: %s", string(prData))
	}

	ghData, err := os.ReadFile(ghLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ghData), "--base main") || !strings.Contains(string(ghData), "--head "+result.IntegrationBranch) {
		t.Fatalf("unexpected gh invocation: %s", string(ghData))
	}

	remoteHeads := gitOutput(t, repo, "ls-remote", "--heads", "origin", result.IntegrationBranch)
	if !strings.Contains(remoteHeads, result.IntegrationBranch) {
		t.Fatalf("expected pushed integration branch, got %q", remoteHeads)
	}
}

func TestRunTaskPackWritesFailureIssueDraft(t *testing.T) {
	root := createPackSkeleton(t)
	repo, _ := setupPackRepoWithPublish(t)
	writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"false"}, nil, nil)

	result := RunTaskPack(context.Background(), root, repo)
	if result.FailedTaskID != "task-a" {
		t.Fatalf("expected failed task-a, got %+v", result)
	}
	if result.IssueDraftPath == "" {
		t.Fatalf("expected issue draft path, got %+v", result)
	}

	issueData, err := os.ReadFile(result.IssueDraftPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(issueData), "Task pack \"Sample Pack\" failed") {
		t.Fatalf("unexpected issue draft: %s", string(issueData))
	}
}

func TestRunTaskPackPreparesBranchesWithoutCheckout(t *testing.T) {
	root := createPackSkeleton(t)
	repo, _ := setupPackRepoWithPublish(t)
	writePackTask(t, filepath.Join(root, "inbox"), "a.md", "task-a", "inbox", "ptolemy/task-a", "sequential", nil, []string{"printf a"}, nil, nil)
	writePackTask(t, filepath.Join(root, "inbox"), "b.md", "task-b", "inbox", "ptolemy/task-b", "sequential", nil, []string{"printf b"}, nil, nil)

	before := gitOutput(t, repo, "branch", "--show-current")
	result := RunTaskPack(context.Background(), root, repo)
	after := gitOutput(t, repo, "branch", "--show-current")
	branchList := gitOutput(t, repo, "branch", "--list", "ptolemy/task-a", "ptolemy/task-b")

	if result.FailedTaskID != "" || len(result.ValidationErrors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if before != after {
		t.Fatalf("expected branch to stay %q, got %q", before, after)
	}
	if !strings.Contains(branchList, "ptolemy/task-a") || !strings.Contains(branchList, "ptolemy/task-b") {
		t.Fatalf("expected prepared branches, got %q", branchList)
	}
	if len(result.PreparedBranches) != 2 {
		t.Fatalf("expected prepared branches map, got %+v", result.PreparedBranches)
	}
	if result.IntegrationBranch == "" {
		t.Fatalf("expected integration branch, got %+v", result)
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

func setupPackRepoWithPublish(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	fakeBin := t.TempDir()
	ghLogPath := filepath.Join(fakeBin, "gh.log")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	mustWriteTaskFile(t, filepath.Join(dir, "README.md"), "repo\n")
	run("add", ".")
	run("commit", "-m", "chore: init repo")
	run("init", "--bare", remote)
	run("remote", "add", "origin", remote)
	run("push", "-u", "origin", "main")

	ghScriptPath := filepath.Join(fakeBin, "gh")
	ghScript := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s\\n' \"$*\" > %s\nprintf '%%s\\n' 'https://example.com/pr/123'\n", shellQuoteForScript(ghLogPath))
	if err := os.WriteFile(ghScriptPath, []byte(ghScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))

	return dir, ghLogPath
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func createFeatureBranchCommit(t *testing.T, dir string, branch string, filename string, content string) {
	t.Helper()
	runGit(t, dir, "checkout", "-b", branch)
	mustWriteTaskFile(t, filepath.Join(dir, filename), content)
	runGit(t, dir, "add", filename)
	runGit(t, dir, "commit", "-m", "feat(test): add "+filename)
	runGit(t, dir, "push", "-u", "origin", branch)
	runGit(t, dir, "checkout", "main")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func shellQuoteForScript(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
