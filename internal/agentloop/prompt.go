package agentloop

import (
	"fmt"
	"strings"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/tasks"
)

func BuildMessages(task tasks.Task, kbFiles []navigator.ContextFile, observations []Observation) []brain.Message {
	return []brain.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`You are the controller-directed Ptolemy agent brain.

Return exactly one JSON object.
The controller executes tools. You only propose one action at a time.

Allowed actions:
- read_file
- write_file
- replace_block
- insert_after
- run_command
- explain
- ask_approval

Rules:
- Never return multiple JSON objects.
- Never return markdown.
- Never return a top-level JSON array.
- Read before editing.
- Include all required fields for the chosen action.
- For read_file, always include path.
- For write_file, include path and content.
- For replace_block, include path, old, and new.
- For insert_after, include path, marker, and content.
- For run_command, include command.
- Prefer replace_block or insert_after for existing files.
- Use explain only when the task is complete and validation should run.
- Keep reasons short and concrete.`),
		},
		{
			Role:    "user",
			Content: buildUserPrompt(task, kbFiles, observations),
		},
	}
}

func buildUserPrompt(task tasks.Task, kbFiles []navigator.ContextFile, observations []Observation) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Task ID: %s\n", task.ID)
	fmt.Fprintf(&b, "Branch: %s\n", task.Branch)
	b.WriteString("Allowed files:\n")
	for _, file := range task.AllowedFiles {
		fmt.Fprintf(&b, "- %s\n", file)
	}

	b.WriteString("\nTask body:\n")
	b.WriteString(task.Body)
	b.WriteString("\n\nKnowledge base:\n")
	if len(kbFiles) == 0 {
		b.WriteString("KB unavailable.\n")
	} else {
		for _, file := range kbFiles {
			fmt.Fprintf(&b, "## %s\n%s\n\n", file.Path, file.Content)
		}
	}

	b.WriteString("Observations:\n")
	if len(observations) == 0 {
		b.WriteString("- none yet\n")
	} else {
		for _, observation := range observations {
			fmt.Fprintf(&b, "- step %d [%s] %s\n", observation.Step, observation.Source, observation.Summary)
		}
	}

	return b.String()
}
