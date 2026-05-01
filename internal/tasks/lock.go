package tasks

import (
	"path/filepath"
	"strings"
)

func Conflicts(a Task, b Task) bool {
	for _, left := range a.AllowedFiles {
		for _, right := range b.AllowedFiles {
			if allowedPathsOverlap(left, right) {
				return true
			}
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

func allowedPathsOverlap(left string, right string) bool {
	leftNorm, leftIsDir := normalizeAllowedPath(left)
	rightNorm, rightIsDir := normalizeAllowedPath(right)

	switch {
	case leftIsDir && rightIsDir:
		return leftNorm == rightNorm || strings.HasPrefix(leftNorm, rightNorm+"/") || strings.HasPrefix(rightNorm, leftNorm+"/")
	case leftIsDir:
		return rightNorm == leftNorm || strings.HasPrefix(rightNorm, leftNorm+"/")
	case rightIsDir:
		return leftNorm == rightNorm || strings.HasPrefix(leftNorm, rightNorm+"/")
	default:
		return leftNorm == rightNorm
	}
}

func normalizeAllowedPath(path string) (string, bool) {
	trimmed := strings.TrimSpace(path)
	isDir := strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, "\\")
	return filepath.ToSlash(filepath.Clean(trimmed)), isDir
}
