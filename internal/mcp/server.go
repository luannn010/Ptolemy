package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

type Server struct {
	client   *WorkerClient
	tools    []Tool
	handlers []ToolHandler
}

func NewServer(client *WorkerClient, toolGroups ...[]Tool) *Server {
	allTools := []Tool{}

	for _, group := range toolGroups {
		allTools = append(allTools, group...)
	}

	return &Server{
		client: client,
		tools:  allTools,
	}
}

func (s *Server) RegisterHandler(handler ToolHandler) {
	s.handlers = append(s.handlers, handler)
}

func (s *Server) Run(input io.Reader, output io.Writer) {
	scanner := bufio.NewScanner(input)
	writer := bufio.NewWriter(output)

	for scanner.Scan() {
		var req RPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			writeResp(writer, RPCResponse{
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
			// MCP notification: no response

		case "tools/list":
			writeResp(writer, RPCResponse{
				ID: req.ID,
				Result: map[string]any{
					"tools": s.tools,
				},
			})

		case "tools/call":
			result, err := s.handleToolCall(req.Params)
			if err != nil {
				writeResp(writer, RPCResponse{
					ID:    req.ID,
					Error: &RPCError{Code: -32000, Message: err.Error()},
				})
				continue
			}

			writeResp(writer, RPCResponse{
				ID:     req.ID,
				Result: result,
			})

		default:
			writeResp(writer, RPCResponse{
				ID:    req.ID,
				Error: &RPCError{Code: -32601, Message: "method not found"},
			})
		}
	}
}

func (s *Server) handleToolCall(raw json.RawMessage) (map[string]any, error) {
	var params ToolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}

	for _, handler := range s.handlers {
		result, handled, err := handler(params.Name, params.Arguments, s.client)
		if handled {
			return result, err
		}
	}

	return nil, fmt.Errorf("unknown tool: %s", params.Name)
}

func writeResp(w *bufio.Writer, resp RPCResponse) {
	resp.JSONRPC = "2.0"
	data, _ := json.Marshal(resp)
	_, _ = w.WriteString(string(data) + "\n")
	_ = w.Flush()
}
