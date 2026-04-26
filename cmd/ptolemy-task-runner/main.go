package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("ptolemy-task-runner ready")

	dirs := []string{
		"docs/tasks/inbox",
		"docs/tasks/active",
		"docs/tasks/split",
		"docs/tasks/done",
		"docs/tasks/failed",
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("Creating directory: %s\n", dir)
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
				os.Exit(1)
			}
		}
	}
}
