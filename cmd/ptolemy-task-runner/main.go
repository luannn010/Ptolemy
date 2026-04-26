package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dirs := []string{
		"docs/tasks/inbox",
		"docs/tasks/active",
		"docs/tasks/split",
		"docs/tasks/done",
		"docs/tasks/failed",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	fmt.Println("ptolemy-task-runner ready")

	tasks, err := filepath.Glob("docs/tasks/inbox/*.md")
	if err != nil {
		fmt.Printf("failed to scan inbox: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("no inbox tasks found")
		return
	}

	fmt.Println("inbox tasks:")
	for _, task := range tasks {
		fmt.Println(task)
	}
}
