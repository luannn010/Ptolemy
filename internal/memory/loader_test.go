package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMemory(t *testing.T) {
	root := t.TempDir()

	globalDir := filepath.Join(root, "global")
	projectDir := filepath.Join(root, "projects", "ptolemy")

	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	globalFile := filepath.Join(globalDir, "agent-rules.md")
	projectFile := filepath.Join(projectDir, "architecture.md")

	if err := os.WriteFile(globalFile, []byte("# Agent Rules\n"), 0644); err != nil {
		t.Fatalf("failed to write global file: %v", err)
	}

	if err := os.WriteFile(projectFile, []byte("# Ptolemy Architecture\n"), 0644); err != nil {
		t.Fatalf("failed to write project file: %v", err)
	}

	mem, err := LoadMemory(root, "ptolemy")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(mem.Global) != 1 {
		t.Fatalf("expected 1 global memory file, got %d", len(mem.Global))
	}

	if len(mem.Project) != 1 {
		t.Fatalf("expected 1 project memory file, got %d", len(mem.Project))
	}

	if mem.Global[globalFile] != "# Agent Rules\n" {
		t.Fatalf("unexpected global memory content: %s", mem.Global[globalFile])
	}

	if mem.Project[projectFile] != "# Ptolemy Architecture\n" {
		t.Fatalf("unexpected project memory content: %s", mem.Project[projectFile])
	}
}

func TestLoadMemoryIgnoresNonMarkdownFiles(t *testing.T) {
	root := t.TempDir()

	globalDir := filepath.Join(root, "global")
	projectDir := filepath.Join(root, "projects", "ptolemy")

	_ = os.MkdirAll(globalDir, 0755)
	_ = os.MkdirAll(projectDir, 0755)

	_ = os.WriteFile(filepath.Join(globalDir, "agent-rules.md"), []byte("# Rules\n"), 0644)
	_ = os.WriteFile(filepath.Join(globalDir, "ignore.txt"), []byte("ignore me"), 0644)

	_ = os.WriteFile(filepath.Join(projectDir, "architecture.md"), []byte("# Architecture\n"), 0644)
	_ = os.WriteFile(filepath.Join(projectDir, "ignore.json"), []byte("{}"), 0644)

	mem, err := LoadMemory(root, "ptolemy")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(mem.Global) != 1 {
		t.Fatalf("expected only markdown global files, got %d", len(mem.Global))
	}

	if len(mem.Project) != 1 {
		t.Fatalf("expected only markdown project files, got %d", len(mem.Project))
	}
}

func TestLoadWorkspaceMemoryPrefersPtolemyContext(t *testing.T) {
	root := t.TempDir()

	ptolemyContext := filepath.Join(root, ".ptolemy", "context")
	legacyContext := filepath.Join(root, "docs", "memory", "projects", "ptolemy")
	if err := os.MkdirAll(ptolemyContext, 0755); err != nil {
		t.Fatalf("failed to create .ptolemy context: %v", err)
	}
	if err := os.MkdirAll(legacyContext, 0755); err != nil {
		t.Fatalf("failed to create legacy context: %v", err)
	}

	ptolemyGuide := filepath.Join(root, ".ptolemy", "PTOLEMY.md")
	ptolemyProjectMap := filepath.Join(ptolemyContext, "project-map.md")
	legacyArchitecture := filepath.Join(legacyContext, "architecture.md")

	if err := os.WriteFile(ptolemyGuide, []byte("# Ptolemy\n"), 0644); err != nil {
		t.Fatalf("failed to write PTOLEMY.md: %v", err)
	}
	if err := os.WriteFile(ptolemyProjectMap, []byte("# Project Map\n"), 0644); err != nil {
		t.Fatalf("failed to write project map: %v", err)
	}
	if err := os.WriteFile(legacyArchitecture, []byte("# Legacy Architecture\n"), 0644); err != nil {
		t.Fatalf("failed to write legacy architecture: %v", err)
	}

	mem, err := LoadWorkspaceMemory(root)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, ok := mem.Project[ptolemyGuide]; !ok {
		t.Fatalf("expected PTOLEMY.md to be loaded")
	}
	if _, ok := mem.Project[ptolemyProjectMap]; !ok {
		t.Fatalf("expected .ptolemy context file to be loaded")
	}
	if _, ok := mem.Project[legacyArchitecture]; ok {
		t.Fatalf("expected docs/memory to be ignored when .ptolemy context exists")
	}
}

func TestLoadWorkspaceMemoryFallsBackToLegacyDocsMemory(t *testing.T) {
	root := t.TempDir()

	globalDir := filepath.Join(root, "docs", "memory", "global")
	projectDir := filepath.Join(root, "docs", "memory", "projects", "ptolemy")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	globalFile := filepath.Join(globalDir, "agent-rules.md")
	projectFile := filepath.Join(projectDir, "architecture.md")

	if err := os.WriteFile(globalFile, []byte("# Agent Rules\n"), 0644); err != nil {
		t.Fatalf("failed to write global file: %v", err)
	}
	if err := os.WriteFile(projectFile, []byte("# Architecture\n"), 0644); err != nil {
		t.Fatalf("failed to write project file: %v", err)
	}

	mem, err := LoadWorkspaceMemory(root)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(mem.Global) != 1 {
		t.Fatalf("expected legacy global memory to load, got %d", len(mem.Global))
	}
	if len(mem.Project) != 1 {
		t.Fatalf("expected legacy project memory to load, got %d", len(mem.Project))
	}
}
