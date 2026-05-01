package clientscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesMemoryFiles(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "node_modules", "x.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ws); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"codebase-map.md", "dependency-map.md", "recent-changes.md"} {
		path := filepath.Join(ws, ".ptolemy", "memory", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}

	codebaseData, err := os.ReadFile(filepath.Join(ws, ".ptolemy", "memory", "codebase-map.md"))
	if err != nil {
		t.Fatal(err)
	}
	codebase := string(codebaseData)
	if !strings.Contains(codebase, "`go.mod`") || !strings.Contains(codebase, "`main.go`") {
		t.Fatalf("unexpected codebase map: %q", codebase)
	}
	if strings.Contains(codebase, "node_modules/x.js") {
		t.Fatalf("excluded directory should not be scanned: %q", codebase)
	}
}
