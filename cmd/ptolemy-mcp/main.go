package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	JSONRPC string    `json:"jsonrpc,omitempty"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func main() {
	workerURL := os.Getenv("PTOLEMY_WORKER_URL")
	if workerURL == "" {
		workerURL = "http://localhost:8080"
	}

	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	for scanner.Scan() {
		var req RPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			writeResp(writer, RPCResponse{
				ID:    req.ID,
				Error: &RPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		switch req.Method {
		case "initialize":
			writeResp(writer, RPCResponse{
				ID: req.ID,
				Result: map[string]any{
					"protocolVersion": "2025-06-18",
					"serverInfo": map[string]any{
						"name":    "ptolemy-mcp",
						"version": "0.1.0",
					},
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
				},
			})

		case "notifications/initialized":
			// no response for notification

		case "tools/list":
			writeResp(writer, RPCResponse{
				ID:     req.ID,
				Result: map[string]any{"tools": tools()},
			})

		case "tools/call":
			resp, err := handleToolCall(workerURL, req.Params)
			if err != nil {
				writeResp(writer, RPCResponse{
					ID:    req.ID,
					Error: &RPCError{Code: -32000, Message: err.Error()},
				})
				continue
			}

			writeResp(writer, RPCResponse{
				ID:     req.ID,
				Result: resp,
			})

		default:
			writeResp(writer, RPCResponse{
				ID:    req.ID,
				Error: &RPCError{Code: -32601, Message: "method not found"},
			})
		}
	}
}

func tools() []map[string]any {
	return []map[string]any{
		tool("ptolemy.execute", "Run a command inside a Ptolemy worker session using tmux.", map[string]any{
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
		tool("ptolemy.read_file", "Read a file from a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		tool("ptolemy.write_file", "Write content to a file in a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path", "content"},
		}),
		tool("ptolemy.list_directory", "List files and folders in a session workspace.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		tool("ptolemy.search_codebase", "Search a session workspace using ripgrep.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"query":      map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "query"},
		}),
		tool("ptolemy.apply_patch", "Apply a basic patch by replacing file content.", map[string]any{
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

func tool(name, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": inputSchema,
	}
}

func handleToolCall(workerURL string, raw json.RawMessage) (map[string]any, error) {
	var params ToolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}

	endpoint := ""
	switch params.Name {
	case "ptolemy.execute":
		endpoint = "/execute"
	case "ptolemy.read_file":
		endpoint = "/file/read"
	case "ptolemy.write_file":
		endpoint = "/file/write"
	case "ptolemy.list_directory":
		endpoint = "/file/list"
	case "ptolemy.search_codebase":
		endpoint = "/file/search"
	case "ptolemy.apply_patch":
		endpoint = "/file/apply"
	default:
		return nil, fmt.Errorf("unknown tool: %s", params.Name)
	}

	body, err := postJSON(workerURL+endpoint, params.Arguments)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(body),
			},
		},
	}, nil
}

func postJSON(url string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("worker error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func writeResp(w *bufio.Writer, resp RPCResponse) {
	resp.JSONRPC = "2.0"
	data, _ := json.Marshal(resp)
	_, _ = w.WriteString(string(data) + "\n")
	_ = w.Flush()
}
