package clientinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeCreatesPtolemyTreeAndConfig(t *testing.T) {
	workspace := t.TempDir()

	result, err := Initialize(Options{
		Workspace:   workspace,
		ServerURL:   "https://ptolemy.example",
		ProjectName: "demo-project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, path := range []string{
		filepath.Join(workspace, ".ptolemy", "context", "architecture.md"),
		filepath.Join(workspace, ".ptolemy", "tasks", "inbox"),
		filepath.Join(workspace, ".ptolemy", "memory", "codebase-map.md"),
		filepath.Join(workspace, ".ptolemy", "cache", "skills"),
		filepath.Join(workspace, ".ptolemy", "client.yaml"),
		filepath.Join(workspace, ".gitignore"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	configData, err := os.ReadFile(filepath.Join(workspace, ".ptolemy", "client.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), `server_url: "https://ptolemy.example"`) {
		t.Fatalf("unexpected client config: %s", string(configData))
	}
	if len(result.Created) == 0 {
		t.Fatalf("expected created files, got %+v", result)
	}
}

func TestInitializeDoesNotOverwriteExistingContextFilesWithoutForce(t *testing.T) {
	workspace := t.TempDir()
	contextPath := filepath.Join(workspace, ".ptolemy", "context")
	if err := os.MkdirAll(contextPath, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(contextPath, "architecture.md")
	if err := os.WriteFile(target, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Initialize(Options{Workspace: workspace})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom\n" {
		t.Fatalf("expected existing file to be preserved, got %q", string(data))
	}
	if len(result.Skipped) == 0 {
		t.Fatalf("expected skipped files, got %+v", result)
	}
}

func TestInitializeOverwritesManagedFilesWithForce(t *testing.T) {
	workspace := t.TempDir()
	contextPath := filepath.Join(workspace, ".ptolemy", "context")
	if err := os.MkdirAll(contextPath, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(contextPath, "architecture.md")
	if err := os.WriteFile(target, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Initialize(Options{Workspace: workspace, Force: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "custom\n" {
		t.Fatalf("expected force to overwrite file, got %q", string(data))
	}
}
