package gittools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_git_prepare_pr_description", "Collect PR template and git context so another model can draft a complete PR description.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy_git_prepare_pr_description":
		body, err := client.Post("/git/prepare-pr-description", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
