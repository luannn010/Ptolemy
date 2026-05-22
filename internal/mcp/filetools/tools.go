package filetools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_read_file", "Read a file from a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"task_session_id": map[string]any{"type": "string"},
				"path":            map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		mcp.NewTool("ptolemy_write_file", "Write content to a file in a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "content"},
		}),
		mcp.NewTool("ptolemy_list_directory", "List files and folders in a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		mcp.NewTool("ptolemy_search_codebase", "Search a session workspace using ripgrep.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"query":      map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "query"},
		}),
		mcp.NewTool("ptolemy_apply_patch", "Apply a basic patch by replacing file content.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "content"},
		}),
		mcp.NewTool("ptolemy_replace_block", "Replace the first exact text block match in a session workspace file.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"old":        map[string]any{"type": "string"},
				"new":        map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "old", "new"},
		}),
		mcp.NewTool("ptolemy_insert_after", "Insert content after the first exact marker match in a session workspace file.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"marker":     map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "marker", "content"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	if path, ok := workerPath(name); ok {
		body, err := client.Post(path, args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}

func workerPath(name string) (string, bool) {
	switch name {
	case "ptolemy_read_file", "ptolemy.read_file":
		return "/file/read", true
	case "ptolemy_write_file", "ptolemy.write_file":
		return "/file/write", true
	case "ptolemy_list_directory", "ptolemy.list_directory":
		return "/file/list", true
	case "ptolemy_search_codebase", "ptolemy.search_codebase":
		return "/file/search", true
	case "ptolemy_apply_patch", "ptolemy.apply_patch":
		return "/file/apply", true
	case "ptolemy_replace_block", "ptolemy.replace_block":
		return "/file/replace_block", true
	case "ptolemy_insert_after", "ptolemy.insert_after":
		return "/file/insert_after", true
	default:
		return "", false
	}
}
