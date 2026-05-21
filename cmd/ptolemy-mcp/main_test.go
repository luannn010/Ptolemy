package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/executortools"
	"github.com/luannn010/ptolemy/internal/mcp/filetools"
	"github.com/luannn010/ptolemy/internal/mcp/gittools"
	"github.com/luannn010/ptolemy/internal/mcp/skilltools"
	"github.com/luannn010/ptolemy/internal/mcp/sessiontools"
)

func TestMCPServerBootAndListTools(t *testing.T) {
	client := mcp.NewWorkerClient("http://localhost:8080")

	server := mcp.NewServer(
		client,
		sessiontools.Tools(),
		executortools.Tools(),
		filetools.Tools(),
		skilltools.Tools(),
		gittools.Tools(),
	)

	server.RegisterHandler(sessiontools.Handle)
	server.RegisterHandler(executortools.Handle)
	server.RegisterHandler(filetools.Handle)
	server.RegisterHandler(skilltools.Handle)
	server.RegisterHandler(gittools.Handle)

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n")
	var output bytes.Buffer

	server.Run(input, &output)

	if !strings.Contains(output.String(), "ptolemy_execute") {
		t.Fatalf("expected execute tool in MCP, got %s", output.String())
	}

	if !strings.Contains(output.String(), "ptolemy_git_generate_pr_body") {
		t.Fatalf("expected git tool in MCP, got %s", output.String())
	}

	if !strings.Contains(output.String(), "ptolemy_list_skills") {
		t.Fatalf("expected list skills tool in MCP, got %s", output.String())
	}
}

func TestFirstNonEmptyTrimsWhitespace(t *testing.T) {
	got := firstNonEmpty("  ", "\t http://127.0.0.1:18080 \n", "fallback")
	if got != "http://127.0.0.1:18080" {
		t.Fatalf("expected trimmed worker URL, got %q", got)
	}
}
