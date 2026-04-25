package executortools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy.execute", "Run a command inside a Ptolemy worker session using tmux.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"command":    map[string]any{"type": "string"},
				"cwd":        map[string]any{"type": "string"},
				"reason":     map[string]any{"type": "string"},
				"timeout":    map[string]any{"type": "integer"},
			},
			"required": []string{"session_id", "command"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy.execute":
		body, err := client.Post("/execute", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
