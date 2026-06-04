package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPChecker_Up(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	chk := NewHTTPChecker("brain", srv.URL, "/v1/models", true)
	c := chk.Check(context.Background())
	if c.Status != StatusUp || c.Name != "brain" {
		t.Fatalf("got %+v, want up/brain", c)
	}
	if !chk.Required() {
		t.Fatal("Required() should be true")
	}
}

func TestHTTPChecker_DownOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewHTTPChecker("brain", srv.URL, "/v1/models", true).Check(context.Background())
	if c.Status != StatusDown || c.Error == "" {
		t.Fatalf("got %+v, want down with error", c)
	}
}

func TestHTTPChecker_DownOnConnRefused(t *testing.T) {
	// Reserve a port then close it so the dial is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := NewHTTPChecker("embedder", url, "/v1/models", true).Check(context.Background())
	if c.Status != StatusDown {
		t.Fatalf("got %+v, want down", c)
	}
}

func TestHTTPChecker_EmptyURL_RequiredDown(t *testing.T) {
	c := NewHTTPChecker("embedder", "", "/v1/models", true).Check(context.Background())
	if c.Status != StatusDown || c.Error != "not configured" {
		t.Fatalf("got %+v, want down/not configured", c)
	}
}

func TestHTTPChecker_EmptyURL_OptionalDisabled(t *testing.T) {
	c := NewHTTPChecker("mcp", "", "/health", false).Check(context.Background())
	if c.Status != StatusDisabled {
		t.Fatalf("got %+v, want disabled", c)
	}
}
