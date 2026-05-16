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

func TestBuildExecutionPlanDependencyOrder(t *testing.T) {
	tasks := []Task{
		{ID: "child", Status: StatusInbox, ExecutionGroup: "sequential", DependsOn: []string{"parent"}},
		{ID: "parent", Status: StatusInbox, ExecutionGroup: "sequential"},
	}

	plan, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Task.ID != "parent" || plan.Steps[1].Task.ID != "child" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestBuildExecutionPlanSkipsNonInbox(t *testing.T) {
	tasks := []Task{
		{ID: "done", Status: StatusCompleted, ExecutionGroup: "sequential"},
		{ID: "todo", Status: StatusInbox, ExecutionGroup: "sequential"},
	}

	plan, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Task.ID != "todo" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestBuildExecutionPlanSortsByExecutionGroup(t *testing.T) {
	tasks := []Task{
		{ID: "parallel", Status: StatusInbox, ExecutionGroup: "parallel"},
		{ID: "sequential", Status: StatusInbox, ExecutionGroup: "sequential"},
	}

	plan, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Steps[0].Task.ID != "sequential" || plan.Steps[1].Task.ID != "parallel" {
		t.Fatalf("unexpected plan order: %+v", plan)
	}
}

func TestBuildExecutionPlanSortsByPriorityWithinGroup(t *testing.T) {
	tasks := []Task{
		{ID: "normal", Status: StatusInbox, ExecutionGroup: "sequential", Priority: "normal"},
		{ID: "urgent", Status: StatusInbox, ExecutionGroup: "sequential", Priority: "urgent"},
	}

	plan, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Steps[0].Task.ID != "urgent" || plan.Steps[1].Task.ID != "normal" {
		t.Fatalf("unexpected plan order: %+v", plan)
	}
}

func TestBuildExecutionPlanRunsFinalLast(t *testing.T) {
	tasks := []Task{
		{ID: "final", Status: StatusInbox, ExecutionGroup: "final"},
		{ID: "seq", Status: StatusInbox, ExecutionGroup: "sequential"},
	}

	plan, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Steps[1].Task.ID != "final" {
		t.Fatalf("expected final task last, got %+v", plan)
	}
}

func TestBuildExecutionPlanReturnsErrorForMissingDependency(t *testing.T) {
	tasks := []Task{
		{ID: "child", Status: StatusInbox, ExecutionGroup: "sequential", DependsOn: []string{"missing"}},
	}

	_, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildExecutionPlanReturnsErrorForDependencyCycle(t *testing.T) {
	tasks := []Task{
		{ID: "a", Status: StatusInbox, ExecutionGroup: "sequential", DependsOn: []string{"b"}},
		{ID: "b", Status: StatusInbox, ExecutionGroup: "sequential", DependsOn: []string{"a"}},
	}

	_, err := BuildExecutionPlan(tasks, map[string]bool{})
	if err == nil {
		t.Fatal("expected error")
	}
}
