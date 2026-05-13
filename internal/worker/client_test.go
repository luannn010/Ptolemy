package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "session-1",
			"name": "test",
			"workspace": "/tmp",
			"status": "open"
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	session, err := client.CreateSession(context.Background(), CreateSessionRequest{
		Name:      "test",
		Workspace: "/tmp",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if session.ID != "session-1" {
		t.Fatalf("expected session-1, got %s", session.ID)
	}
}

func TestRunCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/session-1/commands" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "cmd-1",
			"session_id": "session-1",
			"command": "echo ok",
			"cwd": "/tmp",
			"exit_code": 0,
			"output": "ok\n",
			"error_output": "",
			"duration_ms": 10,
			"created_at": "now"
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	result, err := client.RunCommand(context.Background(), "session-1", RunCommandRequest{
		Command: "echo ok",
		Timeout: 5,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Output != "ok\n" {
		t.Fatalf("expected ok output, got %q", result.Output)
	}
}
