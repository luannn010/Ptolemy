package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func ScanInbox(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	tasks := make([]Task, 0)
	errs := make([]string, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.Name(), readErr))
			continue
		}

		task, parseErr := ParseTaskMarkdown(path, content)
		if parseErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.Name(), parseErr))
			continue
		}
		tasks = append(tasks, task)
	}

	slices.SortFunc(tasks, func(a, b Task) int {
		pa := priorityRank(a.Priority)
		pb := priorityRank(b.Priority)
		if pa != pb {
			return pa - pb
		}
		return strings.Compare(filepath.Base(a.Path), filepath.Base(b.Path))
	})

	if len(errs) > 0 {
		return tasks, fmt.Errorf("scan inbox errors: %s", strings.Join(errs, "; "))
	}
	return tasks, nil
}

func priorityRank(priority string) int {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "urgent":
		return 0
	case "high":
		return 1
	case "low":
		return 3
	default:
		return 2
	}
}
