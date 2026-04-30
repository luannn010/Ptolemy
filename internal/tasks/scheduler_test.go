package tasks

import "testing"

func TestRunnableNoDependencies(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{{ID: "a", Status: StatusInbox}}
	if len(RunnableTasks(tasks, state)) != 1 {
		t.Fatal("expected runnable task")
	}
}

func TestRunnableCompletedDependency(t *testing.T) {
	state := NewMemoryStateStore()
	state.Set("a", StatusCompleted)
	tasks := []Task{{ID: "b", Status: StatusInbox, DependsOn: []string{"a"}}}
	if len(RunnableTasks(tasks, state)) != 1 {
		t.Fatal("expected runnable task with completed dependency")
	}
}

func TestBlockedMissingDependency(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{{ID: "b", Status: StatusInbox, DependsOn: []string{"a"}}}
	if len(BlockedTasks(tasks, state)) != 1 {
		t.Fatal("expected blocked task")
	}
}

func TestBlockedStatusNotRunnable(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{{ID: "b", Status: StatusBlocked}}
	if len(RunnableTasks(tasks, state)) != 0 {
		t.Fatal("blocked status should not be runnable")
	}
}

func TestRunnablePreservesOrder(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{
		{ID: "a", Status: StatusInbox},
		{ID: "b", Status: StatusInbox},
	}
	got := RunnableTasks(tasks, state)
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatal("order not preserved")
	}
}
