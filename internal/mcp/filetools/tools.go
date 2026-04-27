package filetools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy.read_file", "Read a file from a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"task_session_id": map[string]any{"type": "string"},
				"path":            map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		mcp.NewTool("ptolemy.write_file", "Write content to a file in a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "content"},
		}),
		mcp.NewTool("ptolemy.list_directory", "List files and folders in a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		mcp.NewTool("ptolemy.search_codebase", "Search a session workspace using ripgrep.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"query":      map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "query"},
		}),
		mcp.NewTool("ptolemy.apply_patch", "Apply a basic patch by replacing file content.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "content"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy.read_file":
		body, err := client.Post("/file/read", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.write_file":
		body, err := client.Post("/file/write", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.list_directory":
		body, err := client.Post("/file/list", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.search_codebase":
		body, err := client.Post("/file/search", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.apply_patch":
		body, err := client.Post("/file/apply", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
