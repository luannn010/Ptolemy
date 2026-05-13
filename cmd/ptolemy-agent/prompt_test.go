package main

import (
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/navigator"
)

func TestBuildKnowledgeBasePromptIncludesFilesInOrder(t *testing.T) {
	files := []navigator.ContextFile{
		{Path: ".ptolemy/PTOLEMY.md", Content: "guide"},
		{Path: ".ptolemy/kb/PROJECT_MAP.md", Content: "map"},
	}

	prompt := buildKnowledgeBasePrompt(files, nil)
	if !strings.Contains(prompt, "## .ptolemy/PTOLEMY.md\nguide") {
		t.Fatalf("expected PTOLEMY content in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "## .ptolemy/kb/PROJECT_MAP.md\nmap") {
		t.Fatalf("expected project map content in prompt, got %q", prompt)
	}
	if strings.Index(prompt, ".ptolemy/PTOLEMY.md") > strings.Index(prompt, ".ptolemy/kb/PROJECT_MAP.md") {
		t.Fatalf("expected guide before project map, got %q", prompt)
	}
}

