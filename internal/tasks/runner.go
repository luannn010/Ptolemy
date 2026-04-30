package tasks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

type TaskExecutor interface {
	Execute(task Task) error
}

type BatchRunner struct {
	State    StateStore
	Executor TaskExecutor
	MaxBatch int
}

func (r BatchRunner) RunInbox(tasks []Task) error {
	if r.State == nil {
		r.State = NewMemoryStateStore()
	}
	if r.Executor == nil {
		return fmt.Errorf("executor is required")
	}
	for {
		runnable := RunnableTasks(tasks, r.State)
		if len(runnable) == 0 {
			return nil
		}
		batch := PickNonConflictingBatch(runnable, r.MaxBatch)
		if len(batch) == 0 {
			return nil
		}
		for _, task := range batch {
			r.State.Set(task.ID, StatusRunning)
			if err := r.Executor.Execute(task); err != nil {
				r.State.Set(task.ID, StatusFailed)
				return err
			}
			r.State.Set(task.ID, StatusCompleted)
		}
	}
}

type CommandResult struct {
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
}

type TaskRunResult struct {
	TaskID  string
	Success bool
	Results []CommandResult
}

type Runner struct {
	Workspace string
}

func NewRunner(workspace string) *Runner {
	return &Runner{Workspace: workspace}
}

func (r *Runner) RunValidation(ctx context.Context, task Task) TaskRunResult {
	result := TaskRunResult{
		TaskID:  task.ID,
		Success: true,
		Results: make([]CommandResult, 0, len(task.Validation)),
	}

	workspace := r.Workspace
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			result.Success = false
			result.Results = append(result.Results, CommandResult{
				Command:  "",
				ExitCode: 1,
				Stderr:   err.Error(),
			})
			return result
		}
		workspace = cwd
	}

	for _, command := range task.Validation {
		cmd := exec.CommandContext(ctx, "bash", "-lc", command)
		cmd.Dir = workspace

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		exitCode := 0
		if err != nil {
			result.Success = false
			exitCode = 1
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
		}

		result.Results = append(result.Results, CommandResult{
			Command:  command,
			ExitCode: exitCode,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		})

		if err != nil {
			return result
		}
	}

	return result
}
