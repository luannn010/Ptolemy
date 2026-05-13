package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkerClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}

		if r.URL.Path != "/health" {
			t.Fatalf("expected /health, got %s", r.URL.Path)
		}

		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewWorkerClient(server.URL)

	body, err := client.Get("/health")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(body) != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestWorkerClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if payload["hello"] != "world" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewWorkerClient(server.URL)

	body, err := client.Post("/test", map[string]any{
		"hello": "world",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestWorkerClientReturnsErrorOnBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewWorkerClient(server.URL)

	_, err := client.Get("/fail")
	if err == nil {
		t.Fatal("expected error for bad status")
	}
}
