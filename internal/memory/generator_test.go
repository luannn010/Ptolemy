package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIGenerator_ParsesCitationsAndAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("path: %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "llm-x" {
			t.Errorf("model: %v", body["model"])
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "The answer is X [source:c1] [source:c2]."}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := NewOpenAIGenerator(srv.URL, "llm-x", "tok")
	ans, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{
		System: "sys", User: "usr", SourceIDs: []string{"c1", "c2", "c3"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(ans.Text, "X") {
		t.Fatalf("answer text: %q", ans.Text)
	}
	if len(ans.Citations) != 2 || ans.Citations[0] != "c1" || ans.Citations[1] != "c2" {
		t.Fatalf("citations: %v", ans.Citations)
	}
}

func TestOpenAIGenerator_NoCitationsReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"plain answer"}}]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	ans, _ := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{SourceIDs: []string{"c1"}})
	if len(ans.Citations) != 0 {
		t.Fatalf("expected no citations, got %v", ans.Citations)
	}
}

func TestOpenAIGenerator_HallucinatedCitationDropped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"answer [source:hallucinated] [source:c1]"}}]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	ans, _ := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{SourceIDs: []string{"c1"}})
	if len(ans.Citations) != 1 || ans.Citations[0] != "c1" {
		t.Fatalf("hallucinated id must be dropped, got %v", ans.Citations)
	}
}

func TestOpenAIGenerator_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	if _, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{}); err == nil {
		t.Fatalf("expected error on 502")
	}
}

func TestOpenAIGenerator_NoChoicesReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	if _, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{}); err == nil {
		t.Fatalf("expected error when no choices returned")
	}
}

func TestOpenAIGenerator_MalformedJSONErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	if _, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{}); err == nil {
		t.Fatalf("expected JSON decode error")
	}
}

func TestOpenAIGenerator_TransportErrorSurfaces(t *testing.T) {
	g := NewOpenAIGenerator("http://127.0.0.1:1", "m", "")
	if _, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{}); err == nil {
		t.Fatalf("expected transport error against closed port")
	}
}

func TestOpenAIGenerator_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "secret-key")
	_, _ = g.Generate(context.Background(), Query{Text: "q"}, PromptContext{})
	if gotAuth != "Bearer secret-key" {
		t.Fatalf("auth header: %q", gotAuth)
	}
}

func TestOpenAIGenerator_CompleteIncludesGrammar(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"atoms\":[]}"}}]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	_, err := g.Complete(context.Background(), "sys", "usr", CompleteOptions{Grammar: "root ::= \"x\""})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got["grammar"] == nil {
		t.Fatal("expected grammar in complete request")
	}
}

func TestOpenAIGenerator_CompleteNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	if _, err := g.Complete(context.Background(), "sys", "usr", CompleteOptions{}); err == nil {
		t.Fatal("expected no choices error")
	}
}
