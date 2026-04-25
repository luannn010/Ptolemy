package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/inspect"
	"github.com/luannn010/ptolemy/internal/worker"
)

type BrainAction struct {
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Reason  string `json:"reason"`
}

func main() {
	taskFile := flag.String("task-file", "", "markdown task file to execute")
	maxSteps := flag.Int("max-steps", 8, "max agent steps")
	flag.Parse()

	task := strings.Join(flag.Args(), " ")

	if *taskFile != "" {
		data, err := os.ReadFile(*taskFile)
		if err != nil {
			fmt.Printf("failed to read task file: %v\n", err)
			os.Exit(1)
		}
		task = string(data)
	}

	if strings.TrimSpace(task) == "" {
		fmt.Println("usage: ptolemy-agent [--task-file path] [--max-steps 8] <task>")
		os.Exit(1)
	}

	workspace, err := os.Getwd()
	if err != nil {
		fmt.Printf("failed to get workspace: %v\n", err)
		os.Exit(1)
	}

	snapshot := inspect.InspectWorkspace(workspace)

	snapshotJSON, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal workspace snapshot: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	brainClient := brain.NewClient("http://127.0.0.1:8088", "gemma-4-e2b")
	workerClient := worker.NewClient("http://127.0.0.1:8080")

	session, err := workerClient.CreateSession(ctx, worker.CreateSessionRequest{
		Name:        "ptolemy-agent",
		Workspace:   workspace,
		Description: "created by ptolemy-agent",
	})
	if err != nil {
		fmt.Printf("failed to create worker session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("session: %s\n", session.ID)

	observations := []string{}

	for step := 1; step <= *maxSteps; step++ {
		fmt.Printf("\n--- step %d/%d ---\n", step, *maxSteps)

		reply, err := brainClient.Chat(ctx, []brain.Message{
			{
				Role: "system",
				Content: `You are Ptolemy local executor brain.

You must respond in JSON ONLY.

Format:
{
  "action": "run_command | read_file | write_file | explain | ask_approval",
  "command": "<shell command>",
  "path": "<relative file path>",
  "content": "<file content for write_file>",
  "reason": "<short explanation>"
}

Rules:
- Be concise.
- Do not execute anything yourself.
- Use read_file before editing a file.
- Use write_file only after you know the target file content.
- Use run_command for tests, formatting, and validation.
- If dangerous, action = ask_approval.
- If task is complete, action = explain.
- NEVER return markdown.
- NEVER return plain text.
- NEVER include reasoning_content.
`,
			},
			{
				Role: "user",
				Content: fmt.Sprintf(`Workspace snapshot:
%s

Original task:
%s

Observations so far:
%s`, string(snapshotJSON), task, strings.Join(observations, "\n\n")),
			},
		})
		if err != nil {
			fmt.Printf("brain error: %v\n", err)
			os.Exit(1)
		}

		action, err := parseBrainAction(reply)
		if err != nil {
			fmt.Printf("failed to parse brain JSON:\n%s\nerror: %v\n", reply, err)
			os.Exit(1)
		}

		fmt.Printf("brain action: %s\n", action.Action)
		fmt.Printf("reason: %s\n", action.Reason)

		observation := executeAction(ctx, workerClient, session.ID, workspace, action)
		fmt.Println(observation)

		observations = append(observations, observation)

		if action.Action == "explain" || action.Action == "ask_approval" {
			return
		}
	}

	fmt.Println("max steps reached")
}

func executeAction(
	ctx context.Context,
	workerClient *worker.Client,
	sessionID string,
	workspace string,
	action *BrainAction,
) string {
	switch action.Action {
	case "explain":
		return "DONE: " + action.Reason

	case "ask_approval":
		return fmt.Sprintf("APPROVAL REQUIRED: %s command=%s", action.Reason, action.Command)

	case "run_command":
		if strings.TrimSpace(action.Command) == "" {
			return "ERROR: empty command"
		}

		result, err := workerClient.RunCommand(ctx, sessionID, worker.RunCommandRequest{
			Command: action.Command,
			CWD:     workspace,
			Timeout: 120,
		})
		if err != nil {
			return fmt.Sprintf("ERROR running command: %v", err)
		}

		return fmt.Sprintf(
			"COMMAND RESULT\ncommand: %s\nexit_code: %d\noutput:\n%s\nerror_output:\n%s",
			result.Command,
			result.ExitCode,
			result.Output,
			result.ErrorOutput,
		)

	case "read_file":
		if strings.TrimSpace(action.Path) == "" {
			return "ERROR: empty path"
		}

		result, err := workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
		})
		if err != nil {
			return fmt.Sprintf("ERROR reading file: %v", err)
		}

		return fmt.Sprintf("FILE READ\npath: %s\ncontent:\n%s", result.Path, result.Content)

	case "write_file":
		if strings.TrimSpace(action.Path) == "" {
			return "ERROR: empty path"
		}

		result, err := workerClient.WriteFile(ctx, worker.WriteFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
			Content:   action.Content,
		})
		if err != nil {
			return fmt.Sprintf("ERROR writing file: %v", err)
		}

		return fmt.Sprintf("FILE WRITTEN\npath: %s", result.Path)

	default:
		return fmt.Sprintf("ERROR: unknown action %s", action.Action)
	}
}

func parseBrainAction(reply string) (*BrainAction, error) {
	cleaned := strings.TrimSpace(reply)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var action BrainAction
	if err := json.Unmarshal([]byte(cleaned), &action); err != nil {
		return nil, err
	}

	return &action, nil
}
