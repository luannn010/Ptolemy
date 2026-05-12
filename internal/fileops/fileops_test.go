package fileops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndReadFile(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	err := ops.WriteFile("hello.txt", "hello world")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	content, err := ops.ReadFile("hello.txt")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if content != "hello world" {
		t.Fatalf("expected hello world, got %q", content)
	}
}

func TestListDirectory(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("a.txt", "a"); err != nil {
		t.Fatal(err)
	}

	if err := ops.WriteFile("folder/b.txt", "b"); err != nil {
		t.Fatal(err)
	}

	entries, err := ops.ListDirectory(".")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestRejectPathEscape(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	_, err := ops.ReadFile("../secret.txt")
	if err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestSearch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed")
	}

	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("main.go", "package main\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}

	result, err := ops.Search("func main")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Fatalf("expected search result to contain main.go, got %q", result)
	}
}

func TestApplyPatch(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("test.txt", "old"); err != nil {
		t.Fatal(err)
	}

	if err := ops.ApplyPatch("test.txt", "new"); err != nil {
		t.Fatal(err)
	}

	content, err := ops.ReadFile("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if content != "new" {
		t.Fatalf("expected new, got %q", content)
	}
}

func TestReplaceBlockReplacesFirstMatchOnly(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("test.txt", "old\nkeep\nold\n"); err != nil {
		t.Fatal(err)
	}

	if err := ops.ReplaceBlock("test.txt", "old", "new"); err != nil {
		t.Fatal(err)
	}

	content, err := ops.ReadFile("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if content != "new\nkeep\nold\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestReplaceBlockMissingOldText(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("test.txt", "current"); err != nil {
		t.Fatal(err)
	}

	if err := ops.ReplaceBlock("test.txt", "missing", "new"); err == nil {
		t.Fatal("expected missing old block error")
	}
}

func TestInsertAfterMarker(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("test.txt", "before\n// marker\nafter\n"); err != nil {
		t.Fatal(err)
	}

	if err := ops.InsertAfter("test.txt", "// marker", "inserted"); err != nil {
		t.Fatal(err)
	}

	content, err := ops.ReadFile("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if content != "before\n// marker\ninserted\nafter\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestInsertAfterPreservesLeadingNewline(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.WriteFile("test.txt", "marker\n"); err != nil {
		t.Fatal(err)
	}

	if err := ops.InsertAfter("test.txt", "marker", "\ninserted"); err != nil {
		t.Fatal(err)
	}

	content, err := ops.ReadFile("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if content != "marker\ninserted\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestTargetedEditsRejectPathEscape(t *testing.T) {
	baseDir := t.TempDir()
	ops := New(baseDir)

	if err := ops.ReplaceBlock("../secret.txt", "old", "new"); err == nil {
		t.Fatal("expected replace block path escape error")
	}
	if err := ops.InsertAfter("../secret.txt", "marker", "content"); err == nil {
		t.Fatal("expected insert after path escape error")
	}
}

func TestSearchUsesRipgrepFallbackPath(t *testing.T) {
	originalLookPath := lookPath
	originalFallbackPaths := ripgrepFallbackPaths
	t.Cleanup(func() {
		lookPath = originalLookPath
		ripgrepFallbackPaths = originalFallbackPaths
	})

	baseDir := t.TempDir()
	fakeBin := filepath.Join(baseDir, "rg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$6/main.go:1:func main\"\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	lookPath = func(file string) (string, error) {
		return "", fmt.Errorf("%s not found", file)
	}
	ripgrepFallbackPaths = []string{fakeBin}

	result, err := New(baseDir).Search("func main")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Fatalf("expected fallback rg output, got %q", result)
	}
}
