package fileops

import (
	"os/exec"
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
