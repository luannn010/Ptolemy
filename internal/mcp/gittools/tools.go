package gittools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy.git_status", "Get git status for a Ptolemy session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		mcp.NewTool("ptolemy.git_diff", "Get git diff for a Ptolemy session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		mcp.NewTool("ptolemy.git_log", "Get recent git log for a Ptolemy session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		mcp.NewTool("ptolemy.git_checkout", "Checkout an existing git branch.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"branch":     map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "branch"},
		}),
		mcp.NewTool("ptolemy.git_create_branch", "Create and checkout a new git branch.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"branch":     map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "branch"},
		}),
		mcp.NewTool("ptolemy.git_commit", "Create a git commit using a conventional commit message.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"message":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "message"},
		}),
		mcp.NewTool("ptolemy.git_push", "Push a git branch to a remote.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"remote":     map[string]any{"type": "string"},
				"branch":     map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy.git_status":
		body, err := client.Post("/git/status", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.git_diff":
		body, err := client.Post("/git/diff", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.git_log":
		body, err := client.Post("/git/log", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.git_checkout":
		body, err := client.Post("/git/checkout", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.git_create_branch":
		body, err := client.Post("/git/branch", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.git_commit":
		body, err := client.Post("/git/commit", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.git_push":
		body, err := client.Post("/git/push", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
