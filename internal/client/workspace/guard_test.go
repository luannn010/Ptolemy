package clientworkspace

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolvePath_ValidRelativePath(t *testing.T) {
	ws := t.TempDir()
	guard, err := New(ws)
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}

	got, err := guard.ResolvePath("nested/file.txt")
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	want := filepath.Join(ws, "nested", "file.txt")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestResolvePath_RejectsTraversal(t *testing.T) {
	ws := t.TempDir()
	guard, err := New(ws)
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}

	_, err = guard.ResolvePath("../../etc/passwd")
	if err == nil {
		t.Fatal("expected traversal to fail")
	}
	if !errorsContain(err, ErrPathOutsideWorkspace.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolvePath_RejectsAbsoluteOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	guard, err := New(ws)
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}

	outside := filepath.Join(filepath.Dir(ws), "outside.txt")
	_, err = guard.ResolvePath(outside)
	if err == nil {
		t.Fatal("expected outside absolute path to fail")
	}
	if !errorsContain(err, ErrPathOutsideWorkspace.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolvePath_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on Windows without elevated permissions")
	}

	ws := t.TempDir()
	guard, err := New(ws)
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}

	outsideDir := t.TempDir()
	linkPath := filepath.Join(ws, "escape-link")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err = guard.ResolvePath("escape-link/secret.txt")
	if err == nil {
		t.Fatal("expected symlink escape to fail")
	}
	if !errorsContain(err, ErrPathOutsideWorkspace.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func errorsContain(err error, needle string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), needle)
}
