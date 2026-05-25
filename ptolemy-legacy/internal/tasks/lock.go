package tasks

func Conflicts(a Task, b Task) bool {
	set := map[string]struct{}{}
	for _, f := range a.AllowedFiles {
		set[f] = struct{}{}
	}
	for _, f := range b.AllowedFiles {
		if _, ok := set[f]; ok {
			return true
		}
	}
	return false
}

func PickNonConflictingBatch(tasks []Task, max int) []Task {
	out := make([]Task, 0)
	for _, task := range tasks {
		conflict := false
		for _, picked := range out {
			if Conflicts(task, picked) {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}
		out = append(out, task)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}
