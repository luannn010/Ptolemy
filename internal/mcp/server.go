package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
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
	reader := bufio.NewReader(input)
	writer := bufio.NewWriter(output)

	for {
		body, err := readMessage(reader)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			writeResponse(writer, RPCResponse{
				Error: &RPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		var req RPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeResponse(writer, RPCResponse{
				Error: &RPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		resp := s.handleRequest(req)
		if resp == nil {
			continue
		}
		writeResponse(writer, *resp)
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

func (s *Server) handleRequest(req RPCRequest) *RPCResponse {
	switch req.Method {
	case "initialize":
		return &RPCResponse{
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
		}

	case "notifications/initialized":
		return nil

	case "tools/list":
		return &RPCResponse{
			ID: req.ID,
			Result: map[string]any{
				"tools": s.tools,
			},
		}

	case "tools/call":
		result, err := s.handleToolCall(req.Params)
		if err != nil {
			return &RPCResponse{
				ID:    req.ID,
				Error: &RPCError{Code: -32000, Message: err.Error()},
			}
		}

		return &RPCResponse{
			ID:     req.ID,
			Result: result,
		}

	default:
		return &RPCResponse{
			ID:    req.ID,
			Error: &RPCError{Code: -32601, Message: "method not found"},
		}
	}
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && contentLength == -1 && line == "" {
				return nil, io.EOF
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header")
		}

		if strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			length, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || length < 0 {
				return nil, fmt.Errorf("invalid Content-Length")
			}
			contentLength = length
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	return body, nil
}

func writeResponse(w *bufio.Writer, resp RPCResponse) {
	resp.JSONRPC = "2.0"
	data, _ := json.Marshal(resp)
	_, _ = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data))
	_, _ = w.Write(data)
	_ = w.Flush()
}
