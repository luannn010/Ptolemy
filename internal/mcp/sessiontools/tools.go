package sessiontools

import (
	"fmt"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy.create_session", "Create a new Ptolemy worker session.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string"},
				"workspace":   map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
			},
			"required": []string{"name", "workspace"},
		}),
		mcp.NewTool("ptolemy.list_sessions", "List existing Ptolemy worker sessions.", map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		mcp.NewTool("ptolemy.get_session", "Get a Ptolemy worker session by ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		mcp.NewTool("ptolemy.close_session", "Close a Ptolemy worker session.", map[string]any{
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
	case "ptolemy.create_session":
		body, err := client.Post("/sessions", args)
		return mcp.TextResult(body), true, err

	case "ptolemy.list_sessions":
		body, err := client.Get("/sessions")
		return mcp.TextResult(body), true, err

	case "ptolemy.get_session":
		sessionID, ok := args["session_id"].(string)
		if !ok || sessionID == "" {
			return nil, true, fmt.Errorf("session_id is required")
		}

		body, err := client.Get("/sessions/" + sessionID)
		return mcp.TextResult(body), true, err

	case "ptolemy.close_session":
		sessionID, ok := args["session_id"].(string)
		if !ok || sessionID == "" {
			return nil, true, fmt.Errorf("session_id is required")
		}

		body, err := client.Post("/sessions/"+sessionID+"/close", map[string]any{})
		return mcp.TextResult(body), true, err
	}

	return nil, false, nil
}
