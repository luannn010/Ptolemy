package mcp

import (
	"bytes"
	"encoding/json"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ToolHandler func(name string, args map[string]any, client *WorkerClient) (map[string]any, bool, error)

func NewTool(name, description string, inputSchema map[string]any) Tool {
	return Tool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
	}
}

func TextResult(body []byte) map[string]any {
	return ToolResult(string(body), nil, false)
}

func JSONResult(body []byte, isError bool) map[string]any {
	var structured map[string]any
	if err := json.Unmarshal(body, &structured); err == nil {
		return ToolResult(compactJSON(body), structured, isError)
	}

	return ToolResult(string(body), nil, isError)
}

func ToolResult(text string, structured map[string]any, isError bool) map[string]any {
	result := map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text,
			},
		},
	}
	if isError {
		result["isError"] = true
	}
	if structured != nil {
		result["structuredContent"] = structured
	}
	return result
}

func compactJSON(body []byte) string {
	var out bytes.Buffer
	if err := json.Compact(&out, body); err != nil {
		return string(body)
	}
	return out.String()
}
