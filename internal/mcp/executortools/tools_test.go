package executortools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func TestExecutorToolsRegistered(t *testing.T) {
	tools := Tools()

	if len(tools) != 1 {
		t.Fatalf("expected 1 executor tool, got %d", len(tools))
	}

	if tools[0].Name != "ptolemy_execute" {
		t.Fatalf("expected ptolemy_execute, got %s", tools[0].Name)
	}

	properties, ok := tools[0].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected executor tool properties schema, got %#v", tools[0].InputSchema["properties"])
	}

	for _, field := range []string{"title", "purpose", "reasoning_step", "target"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected executor tool property %q to be present", field)
		}
	}

	required, ok := tools[0].InputSchema["required"].([]string)
	if !ok {
		t.Fatalf("expected required list as []string, got %#v", tools[0].InputSchema["required"])
	}

	expectedRequired := map[string]bool{
		"session_id": true,
		"command":    true,
	}
	if len(required) != len(expectedRequired) {
		t.Fatalf("expected only legacy required fields %v, got %v", expectedRequired, required)
	}

	for _, field := range required {
		if !expectedRequired[field] {
			t.Fatalf("unexpected required field %q", field)
		}
		delete(expectedRequired, field)
	}

	if len(expectedRequired) != 0 {
		t.Fatalf("missing required fields: %v", expectedRequired)
	}
}

func TestHandleRoutesExecuteRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/execute" {
			t.Fatalf("unexpected route %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	got, handled, err := Handle("ptolemy_execute", map[string]any{"session_id": "s", "command": "echo hi"}, mcp.NewWorkerClient(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if got["content"] == nil {
		t.Fatalf("expected text result, got %#v", got)
	}
}

func TestHandleUnknownTool(t *testing.T) {
	got, handled, err := Handle("unknown", map[string]any{}, mcp.NewWorkerClient("http://example.com"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected handled=false")
	}
	if got != nil {
		t.Fatalf("expected nil result, got %#v", got)
	}
}
