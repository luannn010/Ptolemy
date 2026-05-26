package fileops

import (
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
