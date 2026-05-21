package sessiontools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func TestSessionToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy.create_session": false,
		"ptolemy.list_sessions":  false,
		"ptolemy.get_session":    false,
		"ptolemy.close_session":  false,
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
		args     map[string]any
		method   string
		path     string
	}{
		{name: "create", toolName: "ptolemy_create_session", args: map[string]any{"name": "n", "workspace": "w"}, method: http.MethodPost, path: "/sessions"},
		{name: "list", toolName: "ptolemy_list_sessions", args: map[string]any{}, method: http.MethodGet, path: "/sessions"},
		{name: "get", toolName: "ptolemy_get_session", args: map[string]any{"session_id": "abc"}, method: http.MethodGet, path: "/sessions/abc"},
		{name: "close", toolName: "ptolemy_close_session", args: map[string]any{"session_id": "abc"}, method: http.MethodPost, path: "/sessions/abc/close"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path {
					t.Fatalf("unexpected route %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte("ok"))
			}))
			defer srv.Close()

			got, handled, err := Handle(tt.toolName, tt.args, mcp.NewWorkerClient(srv.URL))
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

func TestHandleValidatesSessionID(t *testing.T) {
	for _, toolName := range []string{"ptolemy_get_session", "ptolemy_close_session"} {
		_, handled, err := Handle(toolName, map[string]any{}, mcp.NewWorkerClient("http://example.com"))
		if !handled {
			t.Fatalf("expected handled for %s", toolName)
		}
		if err == nil {
			t.Fatalf("expected validation error for %s", toolName)
		}
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
