package fileops

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestResolve_RejectsEmptyBaseDir(t *testing.T) {
	f := &FileOps{BaseDir: ""}
	if _, err := f.Resolve("x"); err == nil {
		t.Fatalf("expected error for empty BaseDir")
	}
}

func TestSearch_RejectsEmptyQuery(t *testing.T) {
	f := New(t.TempDir())
	if _, err := f.Search(""); err == nil {
		t.Fatalf("expected error for empty query")
	}
}

func TestReadFile_NonexistentReturnsError(t *testing.T) {
	f := New(t.TempDir())
	if _, err := f.ReadFile("no-such-file.txt"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestListDirectory_NonexistentReturnsError(t *testing.T) {
	f := New(t.TempDir())
	if _, err := f.ListDirectory("does-not-exist"); err == nil {
		t.Fatalf("expected error for missing dir")
	}
}

func TestWriteFile_CreatesParentDirs(t *testing.T) {
	f := New(t.TempDir())
	if err := f.WriteFile("nested/deep/file.txt", "hi"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := f.ReadFile("nested/deep/file.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("unexpected content %q", out)
	}
}

func TestResolveRipgrep_NotFoundWhenNoRgAndNoFallbacks(t *testing.T) {
	origLook := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = origLook }()

	origFallbacks := ripgrepFallbackPaths
	ripgrepFallbackPaths = []string{}
	defer func() { ripgrepFallbackPaths = origFallbacks }()

	_, err := resolveRipgrep()
	if err == nil {
		t.Fatal("expected error when rg not in PATH and no fallbacks")
	}
	if !strings.Contains(err.Error(), "ripgrep") {
		t.Fatalf("error should mention ripgrep, got: %v", err)
	}
}

func TestResolveRipgrep_FoundInPath(t *testing.T) {
	origLook := lookPath
	lookPath = func(name string) (string, error) {
		if name == "rg" {
			return "/usr/bin/rg", nil
		}
		return "", exec.ErrNotFound
	}
	defer func() { lookPath = origLook }()

	path, err := resolveRipgrep()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/usr/bin/rg" {
		t.Fatalf("expected /usr/bin/rg, got %q", path)
	}
}

func TestResolveRipgrep_FallbackPath(t *testing.T) {
	origLook := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = origLook }()

	// Write a real file that the fallback stat check will find.
	tmp := t.TempDir()
	fakeRg := tmp + "/rg"
	if err := os.WriteFile(fakeRg, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	origFallbacks := ripgrepFallbackPaths
	ripgrepFallbackPaths = []string{fakeRg}
	defer func() { ripgrepFallbackPaths = origFallbacks }()

	path, err := resolveRipgrep()
	if err != nil {
		t.Fatalf("expected fallback path, got error: %v", err)
	}
	if path != fakeRg {
		t.Fatalf("expected %q, got %q", fakeRg, path)
	}
}
