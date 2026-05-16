package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer

	err := Run(context.Background(), Config{
		Args:    []string{"--version"},
		Version: "test-version",
		Stdout:  &stdout,
		Workdir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != "test-version\n" {
		t.Fatalf("stdout = %q, want test-version", stdout.String())
	}
}

func TestRunWorkspace(t *testing.T) {
	var stdout bytes.Buffer
	dir := t.TempDir()

	err := Run(context.Background(), Config{
		Args:    []string{"--workspace"},
		Stdout:  &stdout,
		Workdir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() == "" {
		t.Fatal("expected workspace output")
	}
}

func TestRunHealthDegradedReturnsErrorAndPrintsChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"degraded","service":"workerd","checks":{"mcp":{"enabled":true,"reachable":false,"error":"connection refused","target":"http://127.0.0.1:8081"},"runtime":{"context":"generic","strategy":"static_allowlist_with_fallback","commands":{"go":{"available":true,"path":"/usr/bin/go"},"npm":{"available":true,"path":"/usr/bin/npm"},"python":{"available":false,"error":"not found"}}}}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err := Run(context.Background(), Config{
		Args:   []string{"health"},
		Stdout: &stdout,
		Getenv: func(key string) string {
			if key == "PTOLEMY_WORKER_URL" {
				return server.URL
			}
			return ""
		},
		Workdir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected degraded health to return error")
	}
	out := stdout.String()
	if !strings.Contains(out, "mcp: false") || !strings.Contains(out, "runtime_python: false") {
		t.Fatalf("expected detailed checks in output, got %s", out)
	}
}
