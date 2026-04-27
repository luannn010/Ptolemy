package memory

import (
	"os"
	"path/filepath"
)

type Memory struct {
	Global  map[string]string
	Project map[string]string
}

func LoadMemory(basePath string, project string) (*Memory, error) {
	mem := &Memory{
		Global:  make(map[string]string),
		Project: make(map[string]string),
	}

	globalPath := filepath.Join(basePath, "global")
	if err := loadMarkdownFiles(globalPath, mem.Global); err != nil {
		return nil, err
	}

	projectPath := filepath.Join(basePath, "projects", project)
	if err := loadMarkdownFiles(projectPath, mem.Project); err != nil {
		return nil, err
	}

	return mem, nil
}

func LoadWorkspaceMemory(workspace string) (*Memory, error) {
	if workspace == "" {
		workspace = "."
	}

	ptolemyRoot := filepath.Join(workspace, ".ptolemy")
	contextRoot := filepath.Join(ptolemyRoot, "context")
	if _, err := os.Stat(contextRoot); err == nil {
		mem := &Memory{
			Global:  make(map[string]string),
			Project: make(map[string]string),
		}

		guidePath := filepath.Join(ptolemyRoot, "PTOLEMY.md")
		if data, err := os.ReadFile(guidePath); err == nil {
			mem.Project[guidePath] = string(data)
		}

		if err := loadMarkdownFiles(contextRoot, mem.Project); err != nil {
			return nil, err
		}

		return mem, nil
	}

	return LoadMemory(filepath.Join(workspace, "docs", "memory"), "ptolemy")
}

func loadMarkdownFiles(root string, target map[string]string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".md" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		target[path] = string(data)
		return nil
	})
}
