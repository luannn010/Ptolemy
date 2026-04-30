package tasks

import (
	"bytes"
	"fmt"
	"strconv"
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
	MaxSteps       int
	RequiresApproval bool
	StopOnError    bool
	HasMaxSteps    bool
	HasRequiresApproval bool
	HasStopOnError bool
	Path           string
	Body           string
	Sections       map[string]string
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
		StopOnError:    true,
		Path:           path,
	}

	parts := bytes.SplitN(content, []byte("---"), 3)
	if len(parts) < 3 || strings.TrimSpace(string(parts[0])) != "" {
		return Task{}, fmt.Errorf("missing frontmatter block")
	}

	frontmatter := strings.TrimSpace(string(parts[1]))
	body := strings.TrimLeft(string(parts[2]), "\r\n")
	task.Body = body
	task.Sections = parseTaskSections(body)

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
	if raw, ok := meta["max_steps"]; ok {
		task.HasMaxSteps = true
		task.MaxSteps, err = parseTaskInt("max_steps", raw)
		if err != nil {
			return Task{}, err
		}
	}
	if raw, ok := meta["requires_approval"]; ok {
		task.HasRequiresApproval = true
		task.RequiresApproval, err = parseTaskBool("requires_approval", raw)
		if err != nil {
			return Task{}, err
		}
	}
	if raw, ok := meta["stop_on_error"]; ok {
		task.HasStopOnError = true
		task.StopOnError, err = parseTaskBool("stop_on_error", raw)
		if err != nil {
			return Task{}, err
		}
	}

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
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		return parseInlineList(raw)
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

func parseInlineList(raw string) []string {
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
	if inner == "" {
		return []string{}
	}

	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		item = strings.Trim(item, "\"")
		item = strings.Trim(item, "'")
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
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

func parseTaskInt(field string, raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("invalid %s: empty value", field)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	return parsed, nil
}

func parseTaskBool(field string, raw string) (bool, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s: %q", field, raw)
	}
}

func parseTaskSections(body string) map[string]string {
	sections := map[string]string{}
	current := ""
	lines := strings.Split(body, "\n")
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			current = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if _, ok := sections[current]; !ok {
				sections[current] = ""
			}
			continue
		}
		if current == "" {
			continue
		}
		if sections[current] == "" {
			sections[current] = line
			continue
		}
		sections[current] += "\n" + line
	}

	for key, value := range sections {
		sections[key] = strings.TrimSpace(value)
	}
	return sections
}
