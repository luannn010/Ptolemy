package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPExecutorClientOpenSession(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sess-123","status":"open","workspace":"."}`))
	}))
	defer srv.Close()

	client := NewHTTPExecutorClient(srv.URL)
	id, err := client.OpenSession(context.Background())
	if err != nil {
		t.Fatalf("OpenSession returned error: %v", err)
	}
	if id != "sess-123" {
		t.Fatalf("expected session id sess-123, got %q", id)
	}
	if gotPath != "/sessions" {
		t.Fatalf("expected POST to /sessions, got %s", gotPath)
	}
	if gotBody["name"] == "" || gotBody["name"] == nil {
		t.Fatalf("expected a non-empty session name in request body, got %v", gotBody["name"])
	}
}

func TestHTTPExecutorClientExecute(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"sess-123","command":"go version","exit_code":0,"summary":"go version go1.24","success":true}`))
	}))
	defer srv.Close()

	client := NewHTTPExecutorClient(srv.URL)
	res, err := client.Execute(context.Background(), "sess-123", "go version")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if gotPath != "/execute" {
		t.Fatalf("expected POST to /execute, got %s", gotPath)
	}
	if gotBody["session_id"] != "sess-123" {
		t.Fatalf("expected session_id sess-123, got %v", gotBody["session_id"])
	}
	if gotBody["command"] != "go version" {
		t.Fatalf("expected command 'go version', got %v", gotBody["command"])
	}
	if res.ExitCode != 0 || !res.Success {
		t.Fatalf("expected exit 0 success, got exit=%d success=%v", res.ExitCode, res.Success)
	}
	if res.Summary != "go version go1.24" {
		t.Fatalf("unexpected summary: %q", res.Summary)
	}
}

func TestHTTPExecutorClientExecutePropagatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"session is not open"}`))
	}))
	defer srv.Close()

	client := NewHTTPExecutorClient(srv.URL)
	if _, err := client.Execute(context.Background(), "sess-123", "go version"); err == nil {
		t.Fatal("expected an error when the server returns 500")
	}
}
