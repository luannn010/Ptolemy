package remotetools

import (
	"fmt"
	"time"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_health", "Check the configured Ptolemy worker health endpoint.", map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		mcp.NewTool("ptolemy_execute", "Execute a command through the remote Ptolemy worker /execute endpoint.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"command":         map[string]any{"type": "string"},
				"timeout_seconds": map[string]any{"type": "number"},
			},
			"required": []string{"session_id", "command"},
		}),
		mcp.NewTool("ptolemy_create_session", "Create a Ptolemy worker session over HTTP.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string"},
				"workspace": map[string]any{"type": "string"},
			},
			"required": []string{"name", "workspace"},
		}),
		mcp.NewTool("ptolemy_run_task_file", "Best-effort task execution helper for a remote Ptolemy worker.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"task_file":  map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "task_file"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy_health":
		body, err := client.GetWithTimeout("/health", client.HealthTimeout)
		if err != nil {
			return nil, true, err
		}
		return mcp.JSONResult(body, false), true, nil

	case "ptolemy_execute":
		sessionID, err := requiredString(args, "session_id")
		if err != nil {
			return nil, true, err
		}
		command, err := requiredString(args, "command")
		if err != nil {
			return nil, true, err
		}

		payload := map[string]any{
			"session_id": sessionID,
			"command":    command,
		}

		timeout := client.DefaultTimeout
		if timeoutSeconds, ok := optionalSeconds(args["timeout_seconds"]); ok {
			payload["timeout"] = timeoutSeconds
			timeout = time.Duration(timeoutSeconds+5) * time.Second
		}

		body, err := client.PostWithTimeout("/execute", payload, timeout)
		if err != nil {
			return nil, true, err
		}
		return mcp.JSONResult(body, false), true, nil

	case "ptolemy_create_session":
		nameArg, err := requiredString(args, "name")
		if err != nil {
			return nil, true, err
		}
		workspace, err := requiredString(args, "workspace")
		if err != nil {
			return nil, true, err
		}

		body, err := client.PostWithTimeout("/sessions/", map[string]any{
			"name":      nameArg,
			"workspace": workspace,
		}, client.DefaultTimeout)
		if err != nil {
			return nil, true, err
		}
		return mcp.JSONResult(body, false), true, nil

	case "ptolemy_run_task_file":
		sessionID, err := requiredString(args, "session_id")
		if err != nil {
			return nil, true, err
		}
		taskFile, err := requiredString(args, "task_file")
		if err != nil {
			return nil, true, err
		}

		body, err := client.PostWithTimeout("/agent/run", map[string]any{
			"session_id": sessionID,
			"task_file":  taskFile,
		}, 60*time.Second)
		if err == nil {
			return mcp.JSONResult(body, false), true, nil
		}

		return mcp.ToolResult(
			"The configured worker does not appear to expose POST /agent/run. Current repo docs show /tasks/run-inbox as the available task endpoint, so use the local agent or task-runner workflow for task files.",
			map[string]any{
				"requested_task_file": taskFile,
				"fallback_endpoint":   "/tasks/run-inbox",
				"error":               err.Error(),
			},
			true,
		), true, nil
	}

	return nil, false, nil
}

func requiredString(args map[string]any, key string) (string, error) {
	value, ok := args[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalSeconds(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || number <= 0 {
		return 0, false
	}
	return int(number), true
}
