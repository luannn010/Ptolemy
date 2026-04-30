package tasks

import (
	"path/filepath"
	"slices"
)

type FileConflict struct {
	File    string
	TaskIDs []string
}

func FindAllowedFileConflicts(tasks []Task) []FileConflict {
	filesToTasks := map[string]map[string]struct{}{}

	for _, task := range tasks {
		for _, path := range task.AllowedFiles {
			cleaned := cleanConflictPath(path)
			if _, ok := filesToTasks[cleaned]; !ok {
				filesToTasks[cleaned] = map[string]struct{}{}
			}
			filesToTasks[cleaned][task.ID] = struct{}{}
		}
	}

	conflicts := make([]FileConflict, 0)
	for file, idsSet := range filesToTasks {
		if len(idsSet) < 2 && !filepath.IsAbs(file) {
			continue
		}

		ids := make([]string, 0, len(idsSet))
		for id := range idsSet {
			ids = append(ids, id)
		}
		slices.Sort(ids)

		conflicts = append(conflicts, FileConflict{
			File:    file,
			TaskIDs: ids,
		})
	}

	slices.SortFunc(conflicts, func(a, b FileConflict) int {
		if a.File < b.File {
			return -1
		}
		if a.File > b.File {
			return 1
		}
		return 0
	})

	return conflicts
}

func CanRunTogether(tasks []Task) bool {
	return len(FindAllowedFileConflicts(tasks)) == 0
}

func cleanConflictPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
