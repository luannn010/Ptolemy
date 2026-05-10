package agentloop

import (
	"fmt"
	"strings"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/tasks"
)

func BuildMessages(task tasks.Task, promptContext PromptContext) []brain.Message {
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
- You are executing only the current child task or current direct task.
- Do not solve future child tasks.
- Read before editing.
- Include all required fields for the chosen action.
- For read_file, always include path.
- For write_file, include path and content.
- For replace_block, include path, old, and new.
- For insert_after, include path, marker, and content.
- For run_command, include command.
- If context is insufficient, request a specific file path.
- Do not request the whole repository.
- Stay within allowed_files.
- Prefer replace_block or insert_after for existing files.
- Use explain only when the task is complete and validation should run.
- Keep reasons short and concrete.`),
		},
		{
			Role:    "user",
			Content: buildUserPrompt(task, promptContext),
		},
	}
}

func buildUserPrompt(task tasks.Task, promptContext PromptContext) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Task ID: %s\n", task.ID)
	fmt.Fprintf(&b, "Branch: %s\n", task.Branch)
	if promptContext.ChildTask != nil {
		fmt.Fprintf(&b, "Current child task: %s (%s)\n", promptContext.ChildTask.ID, promptContext.ChildTask.Phase)
	}
	b.WriteString("Allowed files:\n")
	for _, file := range task.AllowedFiles {
		fmt.Fprintf(&b, "- %s\n", file)
	}

	if promptContext.Manifest != nil {
		b.WriteString("\nProcess manifest:\n")
		b.WriteString(renderManifestSlice(*promptContext.Manifest, promptContext.ChildTask))
	}

	b.WriteString("\nTask body:\n")
	b.WriteString(task.Body)
	b.WriteString("\n\nContext:\n")
	if len(promptContext.Context.Files) == 0 {
		b.WriteString("No additional context loaded.\n")
	} else {
		for _, file := range promptContext.Context.Files {
			b.WriteString(file.Content)
			if !strings.HasSuffix(file.Content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("Observations:\n")
	if len(promptContext.Observations) == 0 {
		b.WriteString("- none yet\n")
	} else {
		for _, observation := range promptContext.Observations {
			fmt.Fprintf(&b, "- step %d [%s] %s\n", observation.Step, observation.Source, observation.Summary)
		}
	}

	return b.String()
}

func renderManifestSlice(manifest tasks.ProcessManifest, current *tasks.ProcessChildTask) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Pack ID: %s\n", manifest.PackID))
	builder.WriteString(fmt.Sprintf("Status: %s\n", manifest.Status))
	if current != nil {
		builder.WriteString(fmt.Sprintf("Current child task ID: %s\n", current.ID))
	}
	builder.WriteString("Child sequence:\n")
	for _, child := range manifest.ChildTasks {
		status := child.Status
		if status == "" {
			status = tasks.ProcessStatusPending
		}
		builder.WriteString(fmt.Sprintf("- %s [%s] %s\n", child.ID, status, child.Title))
	}
	return builder.String()
}
