package executortools

import "github.com/luannn010/ptolemy/internal/mcp"

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_execute", "Run a command inside a Ptolemy worker session using tmux.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":     map[string]any{"type": "string"},
				"command":        map[string]any{"type": "string"},
				"cwd":            map[string]any{"type": "string"},
				"reason":         map[string]any{"type": "string"},
				"timeout":        map[string]any{"type": "integer"},
				"title":          map[string]any{"type": "string"},
				"purpose":        map[string]any{"type": "string"},
				"reasoning_step": map[string]any{"type": "string"},
				"target":         map[string]any{"type": "string"},
				"confirm_token":  map[string]any{"type": "string"},
				"pending_id":     map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "command"},
		}),
	}
}

func Handle(name string, args map[string]any, client *mcp.WorkerClient) (map[string]any, bool, error) {
	switch name {
	case "ptolemy_execute":
		body, err := client.Post("/execute", args)
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
