package tasks

import "fmt"

type TaskExecutor interface {
	Execute(task Task) error
}

type Runner struct {
	State    StateStore
	Executor TaskExecutor
	MaxBatch int
}

func (r Runner) RunInbox(tasks []Task) error {
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
