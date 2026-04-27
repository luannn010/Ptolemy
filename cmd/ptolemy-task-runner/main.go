package main

import (
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
	splitDir           = "docs/tasks/split"
	doneDir            = "docs/tasks/done"
	failedDir          = "docs/tasks/failed"
	archiveDir         = "docs/tasks/archive"
	taskRunnerStateDir = ".state/task-runner"
)

type taskClass string

const (
	classSmall  taskClass = "small"
	classMedium taskClass = "medium"
	classLarge  taskClass = "large"
)

type taskQueue string

const (
	queueSplit taskQueue = "split"
	queueInbox taskQueue = "inbox"
)

type pendingTask struct {
	Path           string
	Queue          taskQueue
	Classification taskClass
	MaxSteps       int
}

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

	activePath := uniqueTaskPath(activeDir, filepath.Base(task.Path))
	if err := os.Rename(task.Path, activePath); err != nil {
		return fmt.Errorf("move selected task to active: %w", err)
	}

	logPath := taskLogPath(activePath)

	fmt.Fprintf(out, "Selected task: %s\n", activePath)
	fmt.Fprintf(out, "Queue: %s\n", task.Queue)
	fmt.Fprintf(out, "Classification: %s\n", task.Classification)
	fmt.Fprintf(out, "Max steps: %d\n", task.MaxSteps)
	fmt.Fprintln(out, "Running agent...")

	cmd := exec.Command(goBinary(), "run", "./cmd/ptolemy-agent", "--task-file", activePath, "--max-steps", strconv.Itoa(task.MaxSteps))
	cmdOutput, runErr := cmd.CombinedOutput()

	logContent := cmdOutput
	if runErr != nil {
		logContent = append(logContent, []byte("\n"+runErr.Error()+"\n")...)
	}
	if err := os.WriteFile(logPath, logContent, 0644); err != nil {
		_, _ = moveTask(activePath, failedDir)
		return fmt.Errorf("write task log: %w", err)
	}

	result := "completed"
	targetDir := doneDir
	if runErr != nil {
		result = "failed"
		targetDir = failedDir
	}

	if _, err := moveTask(activePath, targetDir); err != nil {
		return fmt.Errorf("move active task to %s: %w", targetDir, err)
	}

	fmt.Fprintf(out, "Result: %s\n", result)
	fmt.Fprintf(out, "Log: %s\n", logPath)
	return nil
}

func ensureDirs() error {
	dirs := []string{
		inboxDir,
		activeDir,
		splitDir,
		doneDir,
		failedDir,
		archiveDir,
		taskRunnerStateDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	return nil
}

func selectNextTask() (pendingTask, bool, error) {
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
	targetPath := uniqueTaskPath(targetDir, filepath.Base(path))
	if err := os.Rename(path, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
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
