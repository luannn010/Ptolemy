package tasks

import (
	"errors"
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

func TestRunnerRunsTwoIndependentTasks(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := Runner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusInbox}, {ID: "b", Status: StatusInbox}}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestRunnerDependencyOrder(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := Runner{State: state, Executor: exec}
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

func TestRunnerStopsOnError(t *testing.T) {
	exec := &fakeExecutor{errOn: "b"}
	state := NewMemoryStateStore()
	r := Runner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusInbox}, {ID: "b", Status: StatusInbox}}
	if err := r.RunInbox(tasks); err == nil {
		t.Fatal("expected error")
	}
	if s, _ := state.Get("b"); s != StatusFailed {
		t.Fatalf("expected failed status, got %q", s)
	}
}

func TestRunnerDoesNotRunBlockedTask(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := Runner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusBlocked}}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected no calls, got %v", exec.calls)
	}
}

func TestRunnerNoInfiniteLoopWhenNoneRunnable(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := Runner{State: state, Executor: exec}
	tasks := []Task{{ID: "a", Status: StatusInbox, DependsOn: []string{"missing"}}}
	if err := r.RunInbox(tasks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunnerFinalTaskBlockedUntilDepsComplete(t *testing.T) {
	exec := &fakeExecutor{}
	state := NewMemoryStateStore()
	r := Runner{State: state, Executor: exec}
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
