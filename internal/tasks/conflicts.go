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

	for i, left := range tasks {
		for _, leftPath := range left.AllowedFiles {
			leftKey := cleanConflictPath(leftPath)
			if _, ok := filesToTasks[leftKey]; !ok {
				filesToTasks[leftKey] = map[string]struct{}{}
			}
			filesToTasks[leftKey][left.ID] = struct{}{}

			for j := i + 1; j < len(tasks); j++ {
				right := tasks[j]
				for _, rightPath := range right.AllowedFiles {
					if !allowedPathsOverlap(leftPath, rightPath) {
						continue
					}

					rightKey := cleanConflictPath(rightPath)
					if _, ok := filesToTasks[rightKey]; !ok {
						filesToTasks[rightKey] = map[string]struct{}{}
					}
					filesToTasks[leftKey][right.ID] = struct{}{}
					filesToTasks[rightKey][left.ID] = struct{}{}
					filesToTasks[rightKey][right.ID] = struct{}{}
				}
			}
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
	cleaned, isDir := normalizeAllowedPath(path)
	if isDir {
		return cleaned + "/"
	}
	return cleaned
}
