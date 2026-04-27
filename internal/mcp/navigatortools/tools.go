package navigatortools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy.index_workspace", "Create or refresh .ptolemy navigator context and file-tree index.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"workspace":  map[string]any{"type": "string"},
			},
		}),
		mcp.NewTool("ptolemy.read_context", "Read .ptolemy/PTOLEMY.md and .ptolemy/context markdown files.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"workspace":  map[string]any{"type": "string"},
			},
		}),
		mcp.NewTool("ptolemy.start_task_session", "Create a .ptolemy task session with task, notes, files-read, changes, and test-results files.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"workspace":       map[string]any{"type": "string"},
				"task_session_id": map[string]any{"type": "string"},
				"task":            map[string]any{"type": "string"},
			},
			"required": []string{"task_session_id"},
		}),
		mcp.NewTool("ptolemy.append_session_note", "Append a note to a .ptolemy task session.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"workspace":       map[string]any{"type": "string"},
				"task_session_id": map[string]any{"type": "string"},
				"note":            map[string]any{"type": "string"},
			},
			"required": []string{"task_session_id", "note"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy.index_workspace":
		body, err := client.Post("/navigator/index", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.read_context":
		body, err := client.Post("/navigator/context", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.start_task_session":
		body, err := client.Post("/navigator/session/start", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.append_session_note":
		body, err := client.Post("/navigator/session/note", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
