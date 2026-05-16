package tasks

import (
	"bytes"
	"fmt"
	"strings"
)

type Task struct {
	ID             string
	Priority       string
	ParentTask     string
	Owner          string
	Status         string
	Branch         string
	ExecutionGroup string
	DependsOn      []string
	AllowedFiles   []string
	Validation     []string
	Scripts        []string
	Snippets       []string
	Path           string
	Body           string
	PackContext    *TaskPackContext
}

func ParseTaskMarkdown(path string, content []byte) (Task, error) {
	task := Task{
		Priority:       "normal",
		ExecutionGroup: "sequential",
		DependsOn:      []string{},
		Validation:     []string{},
		Scripts:        []string{},
		Snippets:       []string{},
		Path:           path,
	}

	parts := bytes.SplitN(content, []byte("---"), 3)
	if len(parts) < 3 || strings.TrimSpace(string(parts[0])) != "" {
		return Task{}, fmt.Errorf("missing frontmatter block")
	}

	frontmatter := strings.TrimSpace(string(parts[1]))
	body := strings.TrimLeft(string(parts[2]), "\r\n")
	task.Body = body

	meta, err := parseFrontmatter(frontmatter)
	if err != nil {
		return Task{}, err
	}

	task.ID = meta["task_id"]
	task.Priority = withDefault(meta["priority"], task.Priority)
	task.ParentTask = meta["parent_task"]
	task.Owner = meta["owner"]
	task.Status = meta["status"]
	task.Branch = meta["branch"]
	task.ExecutionGroup = withDefault(meta["execution_group"], task.ExecutionGroup)
	task.DependsOn = listOrEmpty(meta, "depends_on")
	task.AllowedFiles = listOrEmpty(meta, "allowed_files")
	task.Validation = listOrEmpty(meta, "validation")
	task.Scripts = listOrEmpty(meta, "scripts")
	task.Snippets = listOrEmpty(meta, "snippets")

	if task.ID == "" {
		return Task{}, fmt.Errorf("missing required field: task_id")
	}
	if task.Status == "" {
		return Task{}, fmt.Errorf("missing required field: status")
	}
	if task.Branch == "" {
		return Task{}, fmt.Errorf("missing required field: branch")
	}
	if len(task.AllowedFiles) == 0 {
		return Task{}, fmt.Errorf("missing required field: allowed_files")
	}

	return task, nil
}

func parseFrontmatter(frontmatter string) (map[string]string, error) {
	meta := map[string]string{}
	currentListKey := ""
	lines := strings.Split(frontmatter, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if currentListKey == "" {
				return nil, fmt.Errorf("invalid frontmatter list item: %q", line)
			}
			item := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if item == "" {
				continue
			}
			if meta[currentListKey] == "" {
				meta[currentListKey] = item
			} else {
				meta[currentListKey] += "\n" + item
			}
			continue
		}
		currentListKey = ""
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid frontmatter line: %q", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		value = strings.Trim(value, "'")
		meta[key] = value
		if value == "" {
			currentListKey = key
		}
	}
	return meta, nil
}

func listOrEmpty(meta map[string]string, key string) []string {
	raw := strings.TrimSpace(meta[key])
	if raw == "" || raw == "[]" {
		return []string{}
	}
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func withDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
