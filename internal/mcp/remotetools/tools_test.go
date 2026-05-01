package remotetools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func TestHandleHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/health" {
			t.Fatalf("expected /health, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"workerd"}`))
	}))
	defer server.Close()

	client := mcp.NewWorkerClient(server.URL)
	result, handled, err := Handle("ptolemy_health", map[string]any{}, client)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !handled {
		t.Fatal("expected health tool to be handled")
	}

	content := result["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(content, `"status":"ok"`) {
		t.Fatalf("expected health payload in result, got %s", content)
	}
}

func TestHandleRunTaskFileFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := mcp.NewWorkerClient(server.URL)
	result, handled, err := Handle("ptolemy_run_task_file", map[string]any{
		"session_id": "session-1",
		"task_file":  "docs/tasks/demo.md",
	}, client)
	if err != nil {
		t.Fatalf("expected fallback result instead of error, got %v", err)
	}
	if !handled {
		t.Fatal("expected task-file tool to be handled")
	}
	if result["isError"] != true {
		t.Fatalf("expected fallback result to be marked as error, got %+v", result)
	}
}
