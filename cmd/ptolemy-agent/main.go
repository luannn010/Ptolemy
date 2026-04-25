package main

import (
	"context"
	"encoding/json"
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
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

func main() {
	task := strings.Join(os.Args[1:], " ")
	if task == "" {
		fmt.Println("usage: ptolemy-agent <task>")
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

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	brainClient := brain.NewClient("http://127.0.0.1:8088", "gemma-4-e2b")

	reply, err := brainClient.Chat(ctx, []brain.Message{
		{
			Role: "system",
			Content: `You are Ptolemy local executor brain.

You must respond in JSON ONLY.

Format:
{
  "action": "run_command | explain | ask_approval",
  "command": "<shell command>",
  "reason": "<short explanation>"
}

Rules:
- Be concise.
- Do not execute anything yourself.
- If safe, action = run_command.
- If dangerous, action = ask_approval.
- If no command is needed, action = explain.
- For Go tests, prefer: go test ./...
- NEVER return markdown.
- NEVER return plain text.
- NEVER include reasoning_content.
`,
		},
		{
			Role: "user",
			Content: fmt.Sprintf(`Workspace snapshot:
%s

User task:
%s`, string(snapshotJSON), task),
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

	switch action.Action {
	case "explain":
		fmt.Println(action.Reason)
		return

	case "ask_approval":
		fmt.Printf("approval required for command: %s\n", action.Command)
		return

	case "run_command":
		if strings.TrimSpace(action.Command) == "" {
			fmt.Println("brain returned empty command")
			os.Exit(1)
		}

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
		fmt.Printf("running: %s\n\n", action.Command)

		result, err := workerClient.RunCommand(ctx, session.ID, worker.RunCommandRequest{
			Command: action.Command,
			CWD:     workspace,
			Timeout: 60,
		})
		if err != nil {
			fmt.Printf("worker error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("exit_code: %d\n", result.ExitCode)
		if result.Output != "" {
			fmt.Println("output:")
			fmt.Print(result.Output)
		}
		if result.ErrorOutput != "" {
			fmt.Println("error_output:")
			fmt.Print(result.ErrorOutput)
		}

	default:
		fmt.Printf("unknown brain action: %s\n", action.Action)
		os.Exit(1)
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
