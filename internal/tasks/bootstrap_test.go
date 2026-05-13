package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapWorkspaceCreatesMissingAssets(t *testing.T) {
	workspace := t.TempDir()

	result, err := BootstrapWorkspace(workspace)
	if err != nil {
		t.Fatalf("BootstrapWorkspace() error = %v", err)
	}

	if result.Workspace == "" {
		t.Fatal("expected resolved workspace path")
	}

	for _, rel := range []string{
		"WORKFLOWS.md",
		"docs/tasks/templates/task-file-template.md",
		"docs/tasks/templates/split-task-template.md",
	} {
		if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestBootstrapWorkspacePreservesExistingFiles(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "WORKFLOWS.md")
	original := []byte("custom workflow\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := BootstrapWorkspace(workspace); err != nil {
		t.Fatalf("BootstrapWorkspace() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("WORKFLOWS.md was overwritten: got %q want %q", string(got), string(original))
	}
}
