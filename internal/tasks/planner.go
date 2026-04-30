package tasks

import (
	"fmt"
	"slices"
	"strings"
)

type Plan struct {
	Runnable         []Task
	Batch            []Task
	Blocked          []Task
	SkippedConflicts []Task
}

func BuildPlan(tasks []Task, state StateStore, maxBatch int) Plan {
	runnable := RunnableTasks(tasks, state)
	batch := PickNonConflictingBatch(runnable, maxBatch)
	blocked := BlockedTasks(tasks, state)
	selected := map[string]struct{}{}
	for _, t := range batch {
		selected[t.ID] = struct{}{}
	}
	skipped := make([]Task, 0)
	for _, t := range runnable {
		if _, ok := selected[t.ID]; ok {
			continue
		}
		for _, s := range batch {
			if Conflicts(t, s) {
				skipped = append(skipped, t)
				break
			}
		}
	}
	return Plan{Runnable: runnable, Batch: batch, Blocked: blocked, SkippedConflicts: skipped}
}

type PlanStep struct {
	Task Task
}

type ExecutionPlan struct {
	Steps []PlanStep
}

func BuildExecutionPlan(tasks []Task, completed map[string]bool) (ExecutionPlan, error) {
	remaining := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == StatusInbox {
			remaining = append(remaining, task)
		}
	}

	done := make(map[string]bool, len(completed))
	for id, ok := range completed {
		if ok {
			done[id] = true
		}
	}

	plan := ExecutionPlan{Steps: make([]PlanStep, 0, len(remaining))}

	for len(remaining) > 0 {
		runnable := make([]Task, 0)
		blockedIDs := make([]string, 0)

		for _, task := range remaining {
			if dependenciesSatisfied(task, done) {
				runnable = append(runnable, task)
			} else {
				blockedIDs = append(blockedIDs, task.ID)
			}
		}

		if len(runnable) == 0 {
			slices.Sort(blockedIDs)
			return ExecutionPlan{}, fmt.Errorf("unresolved task dependencies: %s", strings.Join(blockedIDs, ", "))
		}

		slices.SortFunc(runnable, comparePlanTasks)
		next := runnable[0]
		plan.Steps = append(plan.Steps, PlanStep{Task: next})
		done[next.ID] = true

		filtered := remaining[:0]
		for _, task := range remaining {
			if task.ID != next.ID {
				filtered = append(filtered, task)
			}
		}
		remaining = filtered
	}

	return plan, nil
}

func dependenciesSatisfied(task Task, completed map[string]bool) bool {
	for _, dep := range task.DependsOn {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func comparePlanTasks(a Task, b Task) int {
	ga := executionGroupRank(a.ExecutionGroup)
	gb := executionGroupRank(b.ExecutionGroup)
	if ga != gb {
		return ga - gb
	}

	pa := priorityRank(a.Priority)
	pb := priorityRank(b.Priority)
	if pa != pb {
		return pa - pb
	}

	return strings.Compare(a.ID, b.ID)
}

func executionGroupRank(group string) int {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "sequential":
		return 0
	case "parallel":
		return 1
	case "final":
		return 2
	default:
		return 3
	}
}
