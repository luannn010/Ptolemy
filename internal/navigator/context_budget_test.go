package navigator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssembleContextTrimsKnowledgeBaseByBudget(t *testing.T) {
	root := t.TempDir()
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("IndexWorkspace() error = %v", err)
	}

	writeTestFile(t, root, ".ptolemy/kb/ARCHITECTURE.md", strings.Repeat("# Architecture\n\ncomponent layout\n\n", 200))
	writeTestFile(t, root, ".ptolemy/kb/DECISIONS.md", strings.Repeat("# Decisions\n\ncore decisions\n\n", 200))
	writeTestFile(t, root, ".ptolemy/kb/FEATURE.md", strings.Repeat("# Feature\n\nfeature planner\n\n", 200))

	selection, err := AssembleContext(ContextRequest{
		Workspace:     root,
		TaskBody:      "Inspect the feature planner behavior and summarize architecture decisions.",
		AllowedFiles:  []string{"internal/feature/planner.go"},
		BudgetTokens:  500,
		MaxFileTokens: 200,
	})
	if err != nil {
		t.Fatalf("AssembleContext() error = %v", err)
	}

	if len(selection.Files) == 0 {
		t.Fatal("expected at least one context file")
	}
	if selection.Files[0].Path != ".ptolemy/PTOLEMY.md" {
		t.Fatalf("expected PTOLEMY first, got %s", selection.Files[0].Path)
	}
	if selection.EstimatedTokens > 500 {
		t.Fatalf("estimated tokens = %d, want <= 500", selection.EstimatedTokens)
	}
	if len(selection.TrimmedPaths) == 0 {
		t.Fatal("expected some trimmed paths when budget is tight")
	}
}

func TestAssembleContextIncludesPreviousSummaries(t *testing.T) {
	root := t.TempDir()
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("IndexWorkspace() error = %v", err)
	}

	selection, err := AssembleContext(ContextRequest{
		Workspace:    root,
		TaskBody:     "Continue the implementation.",
		AllowedFiles: []string{"internal/feature/planner.go"},
		BudgetTokens: 700,
		PreviousSummaries: []ContextFile{
			{
				Path:    filepath.ToSlash(".ptolemy/tasks/process/pack/state/001-inspect-result.md"),
				Content: "# 001-inspect\n\n- Status: done\n",
			},
		},
	})
	if err != nil {
		t.Fatalf("AssembleContext() error = %v", err)
	}

	found := false
	for _, file := range selection.Files {
		if strings.Contains(file.Path, "001-inspect-result.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected previous summary file in selection: %+v", selection.IncludedPaths)
	}
}

func TestReadContextPath_HappyPath(t *testing.T) {
	root := t.TempDir()
	content := "hello context"
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, body, err := readContextPath(root, "notes.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != "notes.md" {
		t.Fatalf("expected rel=notes.md, got %q", rel)
	}
	if body != content {
		t.Fatalf("expected body=%q, got %q", content, body)
	}
}

func TestReadContextPath_AbsolutePath(t *testing.T) {
	root := t.TempDir()
	absFile := filepath.Join(root, "abs.txt")
	if err := os.WriteFile(absFile, []byte("abs"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, body, err := readContextPath(root, absFile)
	if err != nil {
		t.Fatalf("unexpected error for absolute path: %v", err)
	}
	if rel != "abs.txt" {
		t.Fatalf("expected rel=abs.txt, got %q", rel)
	}
	if body != "abs" {
		t.Fatalf("expected body=abs, got %q", body)
	}
}

func TestReadContextPath_EmptyRef(t *testing.T) {
	root := t.TempDir()
	if _, _, err := readContextPath(root, "   "); err == nil {
		t.Fatal("expected error for empty ref")
	}
}

func TestReadContextPath_MissingFile(t *testing.T) {
	root := t.TempDir()
	if _, _, err := readContextPath(root, "no-such-file.md"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
