package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	inboxDir           = "docs/tasks/inbox"
	activeDir          = "docs/tasks/active"
	processDir         = "docs/tasks/process"
	splitDir           = "docs/tasks/split"
	doneDir            = "docs/tasks/done"
	failedDir          = "docs/tasks/failed"
	archiveDir         = "docs/tasks/archive"
	taskRunnerStateDir = ".state/task-runner"
	notificationDir    = ".state/task-runner/notifications"
)

type taskClass string

const (
	classSmall  taskClass = "small"
	classMedium taskClass = "medium"
	classLarge  taskClass = "large"
)

type taskQueue string

const (
	queueProcess taskQueue = "process"
	queueActive  taskQueue = "active"
	queueSplit   taskQueue = "split"
	queueInbox   taskQueue = "inbox"
)

type pendingTask struct {
	Path           string
	Queue          taskQueue
	Classification taskClass
	MaxSteps       int
}

type agentRunner func(taskPath string, maxSteps int) ([]byte, error)

var runAgent = runAgentTask

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stdout, "Result: failed\nError: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	if err := ensureDirs(); err != nil {
		return err
	}

	task, ok, err := selectNextTask()
	if err != nil {
		return err
	}

	if !ok {
		fmt.Fprintln(out, "no pending tasks")
		return nil
	}

	activePath, err := moveToActive(task)
	if err != nil {
		return err
	}

	if shouldSplit(task) {
		splitFiles, err := splitLargeTask(activePath)
		if err != nil {
			_, _ = moveTask(activePath, failedDir)
			return err
		}

		archivePath, err := moveTask(activePath, archiveDir)
		if err != nil {
			return fmt.Errorf("archive split parent task: %w", err)
		}

		fmt.Fprintf(out, "Selected task: %s\n", activePath)
		fmt.Fprintf(out, "Queue: %s\n", task.Queue)
		fmt.Fprintf(out, "Classification: %s\n", task.Classification)
		fmt.Fprintln(out, "Result: split")
		fmt.Fprintf(out, "Split tasks: %d\n", len(splitFiles))
		for _, file := range splitFiles {
			fmt.Fprintf(out, "Split: %s\n", file)
		}
		fmt.Fprintf(out, "Archive: %s\n", archivePath)
		return nil
	}

	processPath, err := moveToProcess(activePath)
	if err != nil {
		return err
	}

	logPath := taskLogPath(processPath)

	fmt.Fprintf(out, "Selected task: %s\n", processPath)
	fmt.Fprintf(out, "Queue: %s\n", task.Queue)
	fmt.Fprintf(out, "Classification: %s\n", task.Classification)
	fmt.Fprintf(out, "Max steps: %d\n", task.MaxSteps)
	fmt.Fprintln(out, "Running agent...")

	cmdOutput, runErr := runAgent(processPath, task.MaxSteps)

	logContent := cmdOutput
	if runErr != nil {
		logContent = append(logContent, []byte("\n"+runErr.Error()+"\n")...)
	}
	if err := os.WriteFile(logPath, logContent, 0644); err != nil {
		_, _ = moveTask(processPath, failedDir)
		return fmt.Errorf("write task log: %w", err)
	}

	result := "completed"
	targetDir := doneDir
	if runErr != nil {
		result = "failed"
		targetDir = failedDir
	}

	finalPath, err := moveTask(processPath, targetDir)
	if err != nil {
		return fmt.Errorf("move process task to %s: %w", targetDir, err)
	}

	fmt.Fprintf(out, "Result: %s\n", result)
	fmt.Fprintf(out, "Log: %s\n", logPath)

	if runErr != nil {
		notificationPath, err := writeFailureNotification(finalPath, logPath, runErr)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Notification: %s\n", notificationPath)
		return nil
	}

	archivePath, err := archiveCompletedTask(finalPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Archive: %s\n", archivePath)
	return nil
}

func ensureDirs() error {
	dirs := []string{
		inboxDir,
		activeDir,
		processDir,
		splitDir,
		doneDir,
		failedDir,
		archiveDir,
		taskRunnerStateDir,
		notificationDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	return nil
}

func selectNextTask() (pendingTask, bool, error) {
	processTasks, err := sortedMarkdownTasks(processDir)
	if err != nil {
		return pendingTask{}, false, err
	}
	if len(processTasks) > 0 {
		return buildPendingTask(processTasks[0], queueProcess, 0)
	}

	activeTasks, err := sortedMarkdownTasks(activeDir)
	if err != nil {
		return pendingTask{}, false, err
	}
	if len(activeTasks) > 0 {
		return buildPendingTask(activeTasks[0], queueActive, 0)
	}

	splitTasks, err := sortedMarkdownTasks(splitDir)
	if err != nil {
		return pendingTask{}, false, err
	}
	if len(splitTasks) > 0 {
		return buildPendingTask(splitTasks[0], queueSplit, 4)
	}

	inboxTasks, err := sortedMarkdownTasks(inboxDir)
	if err != nil {
		return pendingTask{}, false, err
	}
	if len(inboxTasks) == 0 {
		return pendingTask{}, false, nil
	}

	return buildPendingTask(inboxTasks[0], queueInbox, 0)
}

func sortedMarkdownTasks(dir string) ([]string, error) {
	tasks, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	sort.Strings(tasks)
	return tasks, nil
}

func buildPendingTask(path string, queue taskQueue, forcedMaxSteps int) (pendingTask, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return pendingTask{}, false, fmt.Errorf("read task %s: %w", path, err)
	}

	classification := classifyTask(string(content))
	maxSteps := forcedMaxSteps
	if maxSteps == 0 {
		maxSteps = stepBudget(classification)
	}

	return pendingTask{
		Path:           path,
		Queue:          queue,
		Classification: classification,
		MaxSteps:       maxSteps,
	}, true, nil
}

func classifyTask(content string) taskClass {
	lower := strings.ToLower(content)
	largeMarkers := []string{
		"multi-file",
		"multiple files",
		"refactor",
		"architecture",
		"pipeline",
		"full implementation",
		"multiple phases",
		"task runner",
		"split",
		"commit flow",
		"many requirements",
	}

	for _, marker := range largeMarkers {
		if strings.Contains(lower, marker) {
			return classLarge
		}
	}

	if len(content) < 1200 {
		return classSmall
	}
	if len(content) < 4000 {
		return classMedium
	}
	return classLarge
}

func stepBudget(classification taskClass) int {
	switch classification {
	case classSmall:
		return 4
	case classMedium:
		return 8
	default:
		return 10
	}
}

func taskLogPath(taskPath string) string {
	base := filepath.Base(taskPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(taskRunnerStateDir, name+"-output.txt")
}

func moveTask(path string, targetDir string) (string, error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}

	targetPath := uniqueTaskPath(targetDir, filepath.Base(path))
	if err := os.Rename(path, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func moveToActive(task pendingTask) (string, error) {
	if task.Queue == queueActive {
		return task.Path, nil
	}
	if task.Queue == queueProcess {
		return task.Path, nil
	}

	activePath, err := moveTask(task.Path, activeDir)
	if err != nil {
		return "", fmt.Errorf("move selected task to active: %w", err)
	}
	return activePath, nil
}

func moveToProcess(activePath string) (string, error) {
	if filepath.Dir(activePath) == processDir {
		return activePath, nil
	}

	processPath, err := moveTask(activePath, processDir)
	if err != nil {
		return "", fmt.Errorf("move active task to process: %w", err)
	}
	return processPath, nil
}

func shouldSplit(task pendingTask) bool {
	if task.Classification != classLarge {
		return false
	}

	return task.Queue == queueInbox || task.Queue == queueActive
}

func splitLargeTask(parentPath string) ([]string, error) {
	content, err := os.ReadFile(parentPath)
	if err != nil {
		return nil, fmt.Errorf("read large task for split: %w", err)
	}

	scopes := splitScopes(string(content))
	if len(scopes) == 0 {
		return nil, errors.New("large task has no deterministic split scopes")
	}

	title := taskTitle(string(content), filepath.Base(parentPath))
	base := strings.TrimSuffix(filepath.Base(parentPath), filepath.Ext(parentPath))
	created := make([]string, 0, len(scopes))

	for i, scope := range scopes {
		name := fmt.Sprintf("%s-part-%03d.md", base, i+1)
		path := uniqueTaskPath(splitDir, name)
		body := fmt.Sprintf(`# %s - Part %03d

This split task is self-contained. Do not read or reference the parent task file.

## Scope
%s

## Rules
- Execute only this split task.
- Do not continue to another split task in the same run.
- Move this split task to done or failed after execution.
`, title, i+1, scope)

		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			return nil, fmt.Errorf("write split task %s: %w", path, err)
		}
		created = append(created, path)
	}

	return created, nil
}

func splitScopes(content string) []string {
	var scopes []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			scopes = append(scopes, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}

	if len(scopes) > 0 {
		return scopes
	}

	for _, block := range strings.Split(content, "\n\n") {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		scopes = append(scopes, trimmed)
	}

	return scopes
}

func taskTitle(content string, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return strings.TrimSuffix(fallback, filepath.Ext(fallback))
}

func archiveCompletedTask(donePath string) (string, error) {
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", err
	}

	archivePath := uniqueTaskPath(archiveDir, filepath.Base(donePath))
	if err := copyFile(donePath, archivePath); err != nil {
		return "", fmt.Errorf("archive completed task: %w", err)
	}

	return archivePath, nil
}

func writeFailureNotification(failedPath string, logPath string, runErr error) (string, error) {
	if err := os.MkdirAll(notificationDir, 0755); err != nil {
		return "", err
	}

	base := strings.TrimSuffix(filepath.Base(failedPath), filepath.Ext(failedPath))
	notificationPath := uniqueTaskPath(notificationDir, base+"-failed.txt")
	content := fmt.Sprintf("Task failed: %s\nLog: %s\nError: %v\n", failedPath, logPath, runErr)
	if err := os.WriteFile(notificationPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write failure notification: %w", err)
	}

	return notificationPath, nil
}

func copyFile(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}

func uniqueTaskPath(dir string, base string) string {
	candidate := filepath.Join(dir, base)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}

	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	stamp := time.Now().UTC().Format("20060102T150405Z")

	for i := 1; ; i++ {
		suffix := stamp
		if i > 1 {
			suffix = fmt.Sprintf("%s-%d", stamp, i)
		}

		candidate = filepath.Join(dir, fmt.Sprintf("%s-%s%s", name, suffix, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func goBinary() string {
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}

	for _, fallback := range []string{"/usr/local/go/bin/go", "/usr/bin/go"} {
		if _, err := os.Stat(fallback); err == nil {
			return fallback
		}
	}

	return "go"
}

func runAgentTask(taskPath string, maxSteps int) ([]byte, error) {
	cmd := exec.Command(goBinary(), "run", "./cmd/ptolemy-agent", "--task-file", taskPath, "--max-steps", strconv.Itoa(maxSteps))
	return cmd.CombinedOutput()
}
