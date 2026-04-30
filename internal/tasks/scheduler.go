package tasks

func RunnableTasks(tasks []Task, state StateStore) []Task {
	out := make([]Task, 0)
	for _, task := range tasks {
		if task.Status != StatusInbox {
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
		if task.Status != StatusInbox {
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
