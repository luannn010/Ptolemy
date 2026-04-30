package tasks

import "testing"

func TestBuildPlanIncludesRunnableAndBlocked(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{
		{ID: "a", Status: StatusInbox},
		{ID: "b", Status: StatusInbox, DependsOn: []string{"a"}},
	}
	plan := BuildPlan(tasks, state, 0)
	if len(plan.Runnable) != 1 || plan.Runnable[0].ID != "a" {
		t.Fatal("unexpected runnable")
	}
	if len(plan.Blocked) != 1 || plan.Blocked[0].ID != "b" {
		t.Fatal("unexpected blocked")
	}
}

func TestBuildPlanSkippedConflicts(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{
		{ID: "a", Status: StatusInbox, AllowedFiles: []string{"x.go"}},
		{ID: "b", Status: StatusInbox, AllowedFiles: []string{"x.go"}},
	}
	plan := BuildPlan(tasks, state, 0)
	if len(plan.Batch) != 1 || len(plan.SkippedConflicts) != 1 {
		t.Fatal("expected one picked and one skipped conflict")
	}
}

func TestBuildPlanHonorsMaxBatchSize(t *testing.T) {
	state := NewMemoryStateStore()
	tasks := []Task{
		{ID: "a", Status: StatusInbox, AllowedFiles: []string{"x.go"}},
		{ID: "b", Status: StatusInbox, AllowedFiles: []string{"y.go"}},
	}
	plan := BuildPlan(tasks, state, 1)
	if len(plan.Batch) != 1 {
		t.Fatal("expected max batch size honored")
	}
}
