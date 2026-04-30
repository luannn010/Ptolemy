package tasks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeExecutor struct {
	calls []string
	errOn string
}

func (f *fakeExecutor) Execute(task Task) error {
	f.calls = append(f.calls, task.ID)
	if task.ID == f.errOn {
		return errors.New("boom")
	}
	return nil
}

func TestBatchRunnerRunsTwoIndependentTasks(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := BatchRunner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusInbox}, {ID: "b", Status: StatusInbox}}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestBatchRunnerDependencyOrder(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := BatchRunner{State: state, Executor: exec}
	tasks := []Task{
		{ID: "child", Status: StatusInbox, DependsOn: []string{"parent"}},
		{ID: "parent", Status: StatusInbox},
	}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.calls[0] != "parent" || exec.calls[1] != "child" {
		t.Fatalf("unexpected order: %v", exec.calls)
	}
}

func TestBatchRunnerStopsOnError(t *testing.T) {
	exec := &fakeExecutor{errOn: "b"}
	state := NewMemoryStateStore()
	r := BatchRunner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusInbox}, {ID: "b", Status: StatusInbox}}
	if err := r.RunInbox(tasks); err == nil {
		t.Fatal("expected error")
	}
	if s, _ := state.Get("b"); s != StatusFailed {
		t.Fatalf("expected failed status, got %q", s)
	}
}

func TestBatchRunnerDoesNotRunBlockedTask(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := BatchRunner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusBlocked}}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected no calls, got %v", exec.calls)
	}
}

func TestBatchRunnerNoInfiniteLoopWhenNoneRunnable(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := BatchRunner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusInbox, DependsOn: []string{"missing"}}}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchRunnerFinalTaskBlockedUntilDepsComplete(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := BatchRunner{State: state, Executor: exec}
	tasks := []Task{
		{ID: "root", Status: StatusInbox},
		{ID: "leaf", Status: StatusInbox, DependsOn: []string{"root"}},
	}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 2 || exec.calls[0] != "root" || exec.calls[1] != "leaf" {
		t.Fatalf("unexpected execution order: %v", exec.calls)
	}
}

func TestRunnableTasksSkipsCompletedStateOverride(t *testing.T) {
	state := NewMemoryStateStore()
	state.Set("a", StatusCompleted)

	tasks := []Task{{ID: "a", Status: StatusInbox}}

	if got := RunnableTasks(tasks, state); len(got) != 0 {
		t.Fatalf("expected completed task to be skipped, got %+v", got)
	}
}

func TestRunnerRunValidationSuccess(t *testing.T) {
	runner := NewRunner("")
	task := Task{
		ID:         "task-1",
		Validation: []string{"printf 'ok'"},
	}

	result := runner.RunValidation(context.Background(), task)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(result.Results) != 1 || result.Results[0].Stdout != "ok" {
		t.Fatalf("unexpected results: %+v", result.Results)
	}
}

func TestRunnerRunValidationFailureStopsFurtherCommands(t *testing.T) {
	runner := NewRunner("")
	task := Task{
		ID: "task-1",
		Validation: []string{
			"echo first && exit 3",
			"echo second",
		},
	}

	result := runner.RunValidation(context.Background(), task)
	if result.Success {
		t.Fatalf("expected failure, got %+v", result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected stop after first failure, got %+v", result.Results)
	}
	if result.Results[0].ExitCode != 3 {
		t.Fatalf("expected exit code 3, got %+v", result.Results[0])
	}
}

func TestRunnerUsesWorkspaceDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(dir)
	task := Task{
		ID:         "task-1",
		Validation: []string{"test -f marker.txt && printf 'present'"},
	}

	result := runner.RunValidation(context.Background(), task)
	if !result.Success || result.Results[0].Stdout != "present" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunnerCapturesStderr(t *testing.T) {
	runner := NewRunner("")
	task := Task{
		ID:         "task-1",
		Validation: []string{"printf 'warn' >&2; exit 2"},
	}

	result := runner.RunValidation(context.Background(), task)
	if result.Success {
		t.Fatalf("expected failure, got %+v", result)
	}
	if result.Results[0].Stderr != "warn" {
		t.Fatalf("expected stderr capture, got %+v", result.Results[0])
	}
}
