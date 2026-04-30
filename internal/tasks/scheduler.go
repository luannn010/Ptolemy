package tasks

import (
	"context"
	"fmt"
)

func effectiveStatus(task Task, state StateStore) string {
	if state != nil {
		if status, ok := state.Get(task.ID); ok {
			return status
		}
	}
	return task.Status
}

func RunnableTasks(tasks []Task, state StateStore) []Task {
	out := make([]Task, 0)
	for _, task := range tasks {
		if effectiveStatus(task, state) != StatusInbox {
			continue
		}
		ok := true
		for _, dep := range task.DependsOn {
			if !state.Completed(dep) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, task)
		}
	}
	return out
}

func BlockedTasks(tasks []Task, state StateStore) []Task {
	out := make([]Task, 0)
	for _, task := range tasks {
		if effectiveStatus(task, state) != StatusInbox {
			continue
		}
		for _, dep := range task.DependsOn {
			if !state.Completed(dep) {
				out = append(out, task)
				break
			}
		}
	}
	return out
}

type Scheduler struct {
	InboxDir  string
	Workspace string
}

type SchedulerResult struct {
	PlannedTaskIDs   []string
	CompletedTaskIDs []string
	FailedTaskID     string
	ValidationErrors []ValidationError
}

func NewScheduler(inboxDir string, workspace string) *Scheduler {
	return &Scheduler{InboxDir: inboxDir, Workspace: workspace}
}

func (s *Scheduler) Run(ctx context.Context) SchedulerResult {
	taskList, err := ScanInbox(s.InboxDir)
	if err != nil {
		return SchedulerResult{
			PlannedTaskIDs:   []string{},
			CompletedTaskIDs: []string{},
			ValidationErrors: []ValidationError{{
				TaskID: "<scan>",
				Field:  "inbox",
				Reason: err.Error(),
			}},
		}
	}

	return runTaskList(ctx, taskList, s.Workspace)
}

func runTaskList(ctx context.Context, taskList []Task, workspace string) SchedulerResult {
	result := SchedulerResult{
		PlannedTaskIDs:   []string{},
		CompletedTaskIDs: []string{},
		ValidationErrors: []ValidationError{},
	}

	result.ValidationErrors = ValidateTasks(taskList)
	if len(result.ValidationErrors) > 0 {
		return result
	}

	plan, err := BuildExecutionPlan(taskList, map[string]bool{})
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			TaskID: "<plan>",
			Field:  "depends_on",
			Reason: err.Error(),
		})
		return result
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
			return result
		}

		runResult := runner.RunValidation(ctx, task)
		if runResult.Success {
			if err := UpdateTaskStatusFile(task.Path, StatusCompleted); err != nil {
				result.FailedTaskID = task.ID
				result.ValidationErrors = append(result.ValidationErrors, ValidationError{
					TaskID: task.ID,
					Field:  "status",
					Reason: err.Error(),
				})
				return result
			}
			result.CompletedTaskIDs = append(result.CompletedTaskIDs, task.ID)
			continue
		}

		_ = UpdateTaskStatusFile(task.Path, StatusFailed)
		result.FailedTaskID = task.ID
		return result
	}

	return result
}

func (r SchedulerResult) Error() error {
	if len(r.ValidationErrors) > 0 {
		return fmt.Errorf("scheduler completed with %d validation errors", len(r.ValidationErrors))
	}
	if r.FailedTaskID != "" {
		return fmt.Errorf("scheduler failed task %s", r.FailedTaskID)
	}
	return nil
}
