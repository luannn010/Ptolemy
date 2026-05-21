package navigatortools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func TestNavigatorToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy.index_workspace":     false,
		"ptolemy.read_context":        false,
		"ptolemy.start_task_session":  false,
		"ptolemy.append_session_note": false,
		"ptolemy.kb_build":            false,
		"ptolemy.kb_read":             false,
		"ptolemy.kb_update":           false,
	}

	for _, tool := range tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("expected tool %s to be registered", name)
		}
	}
}

func TestHandleRoutesRequests(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		path     string
	}{
		{name: "index", toolName: "ptolemy_index_workspace", path: "/navigator/index"},
		{name: "context", toolName: "ptolemy_read_context", path: "/navigator/context"},
		{name: "start session", toolName: "ptolemy_start_task_session", path: "/navigator/session/start"},
		{name: "note", toolName: "ptolemy_append_session_note", path: "/navigator/session/note"},
		{name: "kb build", toolName: "ptolemy_kb_build", path: "/kb/build"},
		{name: "kb read", toolName: "ptolemy_kb_read", path: "/kb/read"},
		{name: "kb update", toolName: "ptolemy_kb_update", path: "/kb/update"},
		{name: "kb reset", toolName: "ptolemy_kb_reset", path: "/kb/reset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tt.path {
					t.Fatalf("unexpected route %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte("ok"))
			}))
			defer srv.Close()

			got, handled, err := Handle(tt.toolName, map[string]any{"session_id": "s"}, mcp.NewWorkerClient(srv.URL))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatal("expected handled=true")
			}
			if got["content"] == nil {
				t.Fatalf("expected text result, got %#v", got)
			}
		})
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
