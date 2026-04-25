package brain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
