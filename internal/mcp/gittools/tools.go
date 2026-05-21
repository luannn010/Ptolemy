package gittools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_git_generate_pr_body", "Generate a local PR body file from the repository PR template and current git metadata.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":  map[string]any{"type": "string"},
				"output_path": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		mcp.NewTool("ptolemy_git_create_pr", "Create a GitHub PR using the generated body file and template-compatible content.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"base":       map[string]any{"type": "string"},
				"head":       map[string]any{"type": "string"},
				"title":      map[string]any{"type": "string"},
				"body_path":  map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy_git_generate_pr_body":
		body, err := client.Post("/git/generate-pr-body", args)
		return mcp.TextResult(body), true, err
	case "ptolemy_git_create_pr":
		body, err := client.Post("/git/create-pr", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
