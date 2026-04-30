package tasks

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
