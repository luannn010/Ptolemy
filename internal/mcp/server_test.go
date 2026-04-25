package mcp

import (
	"bytes"
	"strings"
	"testing"
)

func TestServerInitialize(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")
	server := NewServer(client)

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), `"protocolVersion"`) {
		t.Fatalf("expected initialize response, got %s", output.String())
	}

	if !strings.Contains(output.String(), `"ptolemy-mcp"`) {
		t.Fatalf("expected server name, got %s", output.String())
	}
}

func TestServerToolsList(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")

	tools := []Tool{
		NewTool("ptolemy.test", "test tool", map[string]any{
			"type": "object",
		}),
	}

	server := NewServer(client, tools)

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n")
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), `"ptolemy.test"`) {
		t.Fatalf("expected tool in list, got %s", output.String())
	}
}

func TestServerToolCall(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")

	server := NewServer(client)

	server.RegisterHandler(func(name string, args map[string]any, client *WorkerClient) (map[string]any, bool, error) {
		if name != "ptolemy.test" {
			return nil, false, nil
		}

		return TextResult([]byte(`{"ok":true}`)), true, nil
	})

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptolemy.test","arguments":{}}}` + "\n")
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), `{\"ok\":true}`) {
		t.Fatalf("expected tool result, got %s", output.String())
	}
}

func TestServerUnknownTool(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")
	server := NewServer(client)

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptolemy.unknown","arguments":{}}}` + "\n")
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), `"error"`) {
		t.Fatalf("expected error response, got %s", output.String())
	}
}

func TestServerUnknownMethod(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")
	server := NewServer(client)

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"unknown/method","params":{}}` + "\n")
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), `"method not found"`) {
		t.Fatalf("expected method not found, got %s", output.String())
	}
}
