package brain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestClientChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "Ready."
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "gemma-4-e2b")

	reply, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "You are concise."},
		{Role: "user", Content: "Say ready."},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if reply != "Ready." {
		t.Fatalf("expected Ready., got %s", reply)
	}
}

func TestClientChatReturnsClearTimeoutError(t *testing.T) {
	t.Setenv("PTOLEMY_BRAIN_TIMEOUT_SECONDS", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"late"}}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "gemma-4-e2b")

	_, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Say ready."},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got := err.Error(); got != "brain request timed out after 1s; reduce prompt size, increase timeout, or use a faster model" {
		t.Fatalf("unexpected timeout error: %s", got)
	}

	if timeout := client.Timeout(); timeout != "1s" {
		t.Fatalf("Timeout() = %q, want 1s", timeout)
	}
	_ = os.Unsetenv("PTOLEMY_BRAIN_TIMEOUT_SECONDS")
}
