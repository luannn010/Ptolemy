package filetools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/mcp"
)

func TestHandle_RoutesEveryFileTool(t *testing.T) {
	cases := []struct {
		tool string
		path string
	}{
		{"ptolemy_read_file", "/file/read"},
		{"ptolemy_write_file", "/file/write"},
		{"ptolemy_list_directory", "/file/list"},
		{"ptolemy_search_codebase", "/file/search"},
		{"ptolemy_apply_patch", "/file/apply"},
		{"ptolemy_replace_block", "/file/replace_block"},
		{"ptolemy_insert_after", "/file/insert_after"},
		{"ptolemy.read_file", "/file/read"},
		{"ptolemy.write_file", "/file/write"},
		{"ptolemy.list_directory", "/file/list"},
		{"ptolemy.search_codebase", "/file/search"},
		{"ptolemy.apply_patch", "/file/apply"},
		{"ptolemy.replace_block", "/file/replace_block"},
		{"ptolemy.insert_after", "/file/insert_after"},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			var hit string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit = r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer srv.Close()

			client := mcp.NewWorkerClient(srv.URL)
			res, handled, err := Handle(tc.tool, map[string]any{"path": "p"}, client)
			if err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			if !handled {
				t.Fatalf("expected handled=true for %s", tc.tool)
			}
			if hit != tc.path {
				t.Fatalf("worker path: got %q want %q", hit, tc.path)
			}
			if res == nil {
				t.Fatalf("expected result payload")
			}
		})
	}
}

func TestHandle_UnknownToolNotHandled(t *testing.T) {
	res, handled, err := Handle("not_a_real_tool", nil, mcp.NewWorkerClient("http://127.0.0.1:0"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatalf("unknown tool must report handled=false")
	}
	if res != nil {
		t.Fatalf("unknown tool must return nil payload")
	}
}

func TestHandle_PropagatesWorkerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	_, handled, err := Handle("ptolemy_read_file", map[string]any{"path": "p"}, mcp.NewWorkerClient(srv.URL))
	if !handled {
		t.Fatalf("expected handled=true even on worker error")
	}
	if err == nil {
		t.Fatalf("expected error from worker 500")
	}
}

// Round-trips JSON to make sure the args payload survives, guarding against
// future Handle refactors that drop fields.
func TestHandle_PreservesArgsJSON(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	args := map[string]any{"path": "p", "content": "c"}
	if _, _, err := Handle("ptolemy_write_file", args, mcp.NewWorkerClient(srv.URL)); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if received["path"] != "p" || received["content"] != "c" {
		t.Fatalf("payload not preserved: %v", received)
	}
}
