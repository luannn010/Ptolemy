package inspect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectWorkspaceDetectsGoProject(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	if err := os.Mkdir(filepath.Join(root, "internal"), 0755); err != nil {
		t.Fatalf("failed to create internal dir: %v", err)
	}

	snapshot := InspectWorkspace(root)

	if snapshot.Workspace == "" {
		t.Fatalf("expected workspace to be set")
	}

	if !contains(snapshot.DetectedFiles, "go.mod") {
		t.Fatalf("expected go.mod to be detected, got %v", snapshot.DetectedFiles)
	}

	if !contains(snapshot.DetectedFiles, "internal/") {
		t.Fatalf("expected internal/ to be detected, got %v", snapshot.DetectedFiles)
	}

	if !contains(snapshot.ProjectTypes, "go") {
		t.Fatalf("expected go project type, got %v", snapshot.ProjectTypes)
	}
}

func TestInspectWorkspaceDetectsNodeProject(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write package.json: %v", err)
	}

	snapshot := InspectWorkspace(root)

	if !contains(snapshot.DetectedFiles, "package.json") {
		t.Fatalf("expected package.json to be detected, got %v", snapshot.DetectedFiles)
	}

	if !contains(snapshot.ProjectTypes, "node") {
		t.Fatalf("expected node project type, got %v", snapshot.ProjectTypes)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}

	return false
}
