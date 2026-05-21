package worktreetools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func TestWorktreeToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy.create_worktree": false,
		"ptolemy.list_worktrees":  false,
		"ptolemy.remove_worktree": false,
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
		{name: "create", toolName: "ptolemy_create_worktree", path: "/worktree/create"},
		{name: "list", toolName: "ptolemy_list_worktrees", path: "/worktree/list"},
		{name: "remove", toolName: "ptolemy_remove_worktree", path: "/worktree/remove"},
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
