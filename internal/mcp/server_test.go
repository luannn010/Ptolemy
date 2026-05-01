package mcp

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestServerInitialize(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")
	server := NewServer(client)

	input := strings.NewReader(frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	var output bytes.Buffer

	server.Run(input, &output)

	body := extractFramedBody(t, output.String())

	if !strings.Contains(body, `"protocolVersion"`) {
		t.Fatalf("expected initialize response, got %s", output.String())
	}

	if !strings.Contains(body, `"ptolemy-mcp"`) {
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

	input := strings.NewReader(frame(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var output bytes.Buffer

	server.Run(input, &output)

	body := extractFramedBody(t, output.String())

	if !strings.Contains(body, `"ptolemy.test"`) {
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

	input := strings.NewReader(frame(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptolemy.test","arguments":{}}}`))
	var output bytes.Buffer

	server.Run(input, &output)

	body := extractFramedBody(t, output.String())

	if !strings.Contains(body, `{\"ok\":true}`) {
		t.Fatalf("expected tool result, got %s", output.String())
	}
}

func TestServerUnknownTool(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")
	server := NewServer(client)

	input := strings.NewReader(frame(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptolemy.unknown","arguments":{}}}`))
	var output bytes.Buffer

	server.Run(input, &output)

	body := extractFramedBody(t, output.String())

	if !strings.Contains(body, `"error"`) {
		t.Fatalf("expected error response, got %s", output.String())
	}
}

func TestServerUnknownMethod(t *testing.T) {
	client := NewWorkerClient("http://localhost:8080")
	server := NewServer(client)

	input := strings.NewReader(frame(`{"jsonrpc":"2.0","id":1,"method":"unknown/method","params":{}}`))
	var output bytes.Buffer

	server.Run(input, &output)

	body := extractFramedBody(t, output.String())

	if !strings.Contains(body, `"method not found"`) {
		t.Fatalf("expected method not found, got %s", output.String())
	}
}

func TestReadMessageAndWriteResponseFraming(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	body, err := readMessage(bufio.NewReader(strings.NewReader(frame(payload))))
	if err != nil {
		t.Fatalf("expected framed message to parse, got %v", err)
	}
	if string(body) != payload {
		t.Fatalf("unexpected framed body: %s", string(body))
	}

	var output bytes.Buffer
	writeResponse(bufio.NewWriter(&output), RPCResponse{
		ID:     1,
		Result: map[string]any{"ok": true},
	})

	framed := output.String()
	if !strings.HasPrefix(framed, "Content-Length: ") {
		t.Fatalf("expected Content-Length header, got %q", framed)
	}
	if !strings.Contains(framed, "\r\n\r\n") {
		t.Fatalf("expected framed separator, got %q", framed)
	}
}

func frame(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}

func extractFramedBody(t *testing.T, framed string) string {
	t.Helper()

	parts := strings.SplitN(framed, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("expected framed output, got %q", framed)
	}

	return parts[1]
}
