package worktreetools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_create_worktree", "Create a new isolated git worktree for a session.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
				"branch":     map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "name"},
		}),
		mcp.NewTool("ptolemy_list_worktrees", "List git worktrees for a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		mcp.NewTool("ptolemy_remove_worktree", "Remove an isolated git worktree by name.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "name"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy_create_worktree":
		body, err := client.Post("/worktree/create", args)
		return mcp.TextResult(body), true, err

	case "ptolemy_list_worktrees":
		body, err := client.Post("/worktree/list", args)
		return mcp.TextResult(body), true, err

	case "ptolemy_remove_worktree":
		body, err := client.Post("/worktree/remove", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
