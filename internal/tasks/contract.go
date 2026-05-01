package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AgentModeBoundedMarkdownContract = "bounded_markdown_contract"

var requiredTaskContractSections = []string{
	"Goal",
	"Scope",
	"Constraints",
	"Inputs",
	"Execution Steps",
	"Acceptance Checks",
	"Failure / Escalation",
	"Done When",
}

func usesBoundedMarkdownContract(task Task) bool {
	return task.PackContext != nil && task.PackContext.AgentMode == AgentModeBoundedMarkdownContract
}

func BuildTaskExecutionContract(task Task) (string, error) {
	if !usesBoundedMarkdownContract(task) {
		return task.Body, nil
	}

	var builder strings.Builder
	builder.WriteString("# Ptolemy Task Execution Contract\n\n")
	builder.WriteString(fmt.Sprintf("- Pack: `%s`\n", task.PackContext.PackID))
	builder.WriteString(fmt.Sprintf("- Task ID: `%s`\n", task.ID))
	builder.WriteString(fmt.Sprintf("- Branch: `%s`\n", task.Branch))
	builder.WriteString(fmt.Sprintf("- Max steps: `%d`\n", task.MaxSteps))
	builder.WriteString(fmt.Sprintf("- Requires approval: `%t`\n", task.RequiresApproval))
	builder.WriteString(fmt.Sprintf("- Stop on error: `%t`\n", task.StopOnError))
	builder.WriteString("- Allowed files:\n")
	for _, path := range task.AllowedFiles {
		builder.WriteString(fmt.Sprintf("  - `%s`\n", path))
	}
	builder.WriteString("- Validation commands:\n")
	for _, command := range task.Validation {
		builder.WriteString(fmt.Sprintf("  - `%s`\n", command))
	}
	if len(task.PackContext.GlobalConstraints) > 0 {
		builder.WriteString("- Global constraints:\n")
		for _, constraint := range task.PackContext.GlobalConstraints {
			builder.WriteString(fmt.Sprintf("  - %s\n", constraint))
		}
	}
	if len(task.Scripts) > 0 || len(task.Snippets) > 0 {
		builder.WriteString("- Referenced pack assets are embedded below.\n")
		builder.WriteString("- Do not call read_file on task-scripts/... or snippets/... paths; treat their embedded contents as the source of truth.\n")
	}
	builder.WriteString("\n")

	for _, heading := range requiredTaskContractSections {
		builder.WriteString(fmt.Sprintf("## %s\n", heading))
		builder.WriteString(task.Sections[heading])
		builder.WriteString("\n\n")
	}

	if len(task.Scripts) > 0 {
		builder.WriteString("## Referenced Task Scripts\n")
		for _, ref := range task.Scripts {
			content, err := loadPackReference(task, ref)
			if err != nil {
				return "", err
			}
			builder.WriteString(fmt.Sprintf("### %s\n", ref))
			builder.WriteString("```md\n")
			builder.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				builder.WriteString("\n")
			}
			builder.WriteString("```\n\n")
		}
	}

	if len(task.Snippets) > 0 {
		builder.WriteString("## Referenced Snippets\n")
		for _, ref := range task.Snippets {
			content, err := loadPackReference(task, ref)
			if err != nil {
				return "", err
			}
			builder.WriteString(fmt.Sprintf("### %s\n", ref))
			builder.WriteString("```text\n")
			builder.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				builder.WriteString("\n")
			}
			builder.WriteString("```\n\n")
		}
	}

	builder.WriteString("## Execution Rules\n")
	builder.WriteString("- Modify only the allowed files.\n")
	builder.WriteString("- Treat referenced scripts and snippets as exact inputs.\n")
	builder.WriteString("- Run the validation commands after edits.\n")
	builder.WriteString("- If blocked or unsafe, stop and explain why.\n")
	return builder.String(), nil
}

func loadPackReference(task Task, ref string) (string, error) {
	if task.PackContext == nil {
		return "", fmt.Errorf("pack context is required to load %s", ref)
	}
	path := filepath.Join(task.PackContext.Root, filepath.FromSlash(ref))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read pack reference %s: %w", ref, err)
	}
	return string(data), nil
}
