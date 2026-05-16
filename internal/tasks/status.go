package tasks

import (
	"fmt"
	"os"
	"strings"
)

func UpdateTaskStatusFile(path string, status string) error {
	if !isAllowedStatus(status) {
		return fmt.Errorf("invalid status: %s", status)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(content)
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "" {
		return fmt.Errorf("missing frontmatter block")
	}

	lines := strings.Split(parts[1], "\n")
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "status:") {
			prefix := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = prefix + "status: " + status
			replaced = true
			break
		}
	}
	if !replaced {
		return fmt.Errorf("frontmatter missing status field")
	}

	updated := parts[0] + "---" + strings.Join(lines, "\n") + "---" + parts[2]
	return os.WriteFile(path, []byte(updated), 0o644)
}
