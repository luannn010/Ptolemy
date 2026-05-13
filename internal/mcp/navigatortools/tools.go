package navigatortools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_index_workspace", "Create or refresh .ptolemy navigator context and file-tree index.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"workspace":  map[string]any{"type": "string"},
			},
		}),
		mcp.NewTool("ptolemy_read_context", "Read .ptolemy/PTOLEMY.md and .ptolemy/context markdown files.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"workspace":  map[string]any{"type": "string"},
			},
		}),
		mcp.NewTool("ptolemy_start_task_session", "Create a .ptolemy task session with task, notes, files-read, changes, and test-results files.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"workspace":       map[string]any{"type": "string"},
				"task_session_id": map[string]any{"type": "string"},
				"task":            map[string]any{"type": "string"},
			},
			"required": []string{"task_session_id"},
		}),
		mcp.NewTool("ptolemy_append_session_note", "Append a note to a .ptolemy task session.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"workspace":       map[string]any{"type": "string"},
				"task_session_id": map[string]any{"type": "string"},
				"note":            map[string]any{"type": "string"},
			},
			"required": []string{"task_session_id", "note"},
		}),
		mcp.NewTool("ptolemy_kb_build", "Create or refresh the canonical .ptolemy/kb knowledge base and compatibility artifacts.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"workspace":  map[string]any{"type": "string"},
			},
		}),
		mcp.NewTool("ptolemy_kb_read", "Read .ptolemy/PTOLEMY.md and .ptolemy/kb markdown files.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"workspace":  map[string]any{"type": "string"},
			},
		}),
		mcp.NewTool("ptolemy_kb_update", "Update the canonical .ptolemy/kb knowledge base for changed files and optional task-pack metadata.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":          map[string]any{"type": "string"},
				"workspace":           map[string]any{"type": "string"},
				"changed_files":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"pack_id":             map[string]any{"type": "string"},
				"completed_task_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"commit_sha":          map[string]any{"type": "string"},
			},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy_index_workspace":
		body, err := client.Post("/navigator/index", args)
		return mcp.TextResult(body), true, err

	case "ptolemy_read_context":
		body, err := client.Post("/navigator/context", args)
		return mcp.TextResult(body), true, err

	case "ptolemy_start_task_session":
		body, err := client.Post("/navigator/session/start", args)
		return mcp.TextResult(body), true, err

	case "ptolemy_append_session_note":
		body, err := client.Post("/navigator/session/note", args)
		return mcp.TextResult(body), true, err
	case "ptolemy_kb_build":
		body, err := client.Post("/kb/build", args)
		return mcp.TextResult(body), true, err
	case "ptolemy_kb_read":
		body, err := client.Post("/kb/read", args)
		return mcp.TextResult(body), true, err
	case "ptolemy_kb_update":
		body, err := client.Post("/kb/update", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
