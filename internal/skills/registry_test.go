package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRegistryRejectsParentTraversalPath(t *testing.T) {
	_, err := NewRegistry(t.TempDir(), "../skills")
	if err == nil {
		t.Fatal("expected path validation error")
	}
}

func TestRegistryDefaultsToPtolemyServerSkillsWhenDocsSkillsMissing(t *testing.T) {
	baseDir := t.TempDir()

	registry, err := NewRegistry(baseDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(baseDir, ".ptolemy", "server", "skills")
	if registry.root != want {
		t.Fatalf("root = %q, want %q", registry.root, want)
	}
}

func TestRegistryPrefersDocsSkillsWhenPresent(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "docs", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	registry, err := NewRegistry(baseDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(baseDir, "docs", "skills")
	if registry.root != want {
		t.Fatalf("root = %q, want %q", registry.root, want)
	}
}

func TestRegistryGetReturnsNotFound(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".ptolemy", "server", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	registry, err := NewRegistry(baseDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = registry.Get("missing-skill")
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestRegistryListAndGetSkill(t *testing.T) {
	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, ".ptolemy", "server", "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "example.md"), []byte("# Example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := NewRegistry(baseDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skills, err := registry.List()
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(skills) != 1 || skills[0].ID != "example" {
		t.Fatalf("unexpected skills: %+v", skills)
	}

	doc, err := registry.Get("example")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if doc.Content != "# Example\n" {
		t.Fatalf("unexpected content: %q", doc.Content)
	}
}
