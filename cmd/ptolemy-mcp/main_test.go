package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/remotetools"
)

func TestMCPServerBootAndListTools(t *testing.T) {
	client := mcp.NewWorkerClient("http://localhost:8080")

	server := mcp.NewServer(
		client,
		remotetools.Tools(),
	)

	server.RegisterHandler(remotetools.Handle)

	request := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	input := strings.NewReader("Content-Length: " + strconv.Itoa(len(request)) + "\r\n\r\n" + request)
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), "ptolemy_execute") {
		t.Fatalf("expected execute tool in MCP, got %s", output.String())
	}

	if !strings.Contains(output.String(), "ptolemy_health") {
		t.Fatalf("expected health tool in MCP, got %s", output.String())
	}
}
