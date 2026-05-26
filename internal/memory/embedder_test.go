package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbedder_BatchesAndUnpacks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/embeddings") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "test-model" || len(body.Input) != 2 {
			t.Errorf("unexpected body %+v", body)
		}
		resp := map[string]any{
			"data": []map[string]any{
				{"embedding": []float64{0.1, 0.2, 0.3}},
				{"embedding": []float64{0.4, 0.5, 0.6}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "test-model", "key-x")
	vecs, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 || vecs[0][0] != 0.1 || vecs[1][2] != 0.6 {
		t.Fatalf("unexpected vectors: %+v", vecs)
	}
}

func TestOpenAIEmbedder_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0]}]}`))
	}))
	defer srv.Close()
	e := NewOpenAIEmbedder(srv.URL, "m", "secret")
	if _, err := e.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization header: %q", gotAuth)
	}
}

func TestOpenAIEmbedder_NoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0]}]}`))
	}))
	defer srv.Close()
	e := NewOpenAIEmbedder(srv.URL, "m", "")
	if _, err := e.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header for empty key, got %q", gotAuth)
	}
}

func TestOpenAIEmbedder_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	e := NewOpenAIEmbedder(srv.URL, "m", "")
	if _, err := e.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatalf("expected error on 500")
	}
}

func TestOpenAIEmbedder_TrailingSlashStripped(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0]}]}`))
	}))
	defer srv.Close()
	e := NewOpenAIEmbedder(srv.URL+"/", "m", "")
	if _, err := e.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/embeddings" {
		t.Fatalf("expected single slash, got %q", gotPath)
	}
}
