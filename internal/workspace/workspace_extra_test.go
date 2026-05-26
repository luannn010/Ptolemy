package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_EmptyUsesCWD(t *testing.T) {
	got, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\") error: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty CWD")
	}
}

func TestResolve_NonexistentPathErrors(t *testing.T) {
	if _, err := Resolve(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatalf("expected error for nonexistent path")
	}
}

func TestResolve_FilePathRejected(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(file); err == nil {
		t.Fatalf("expected error when path is a file, not directory")
	}
}

func TestResolve_DirectoryReturnsClean(t *testing.T) {
	dir := t.TempDir()
	got, err := Resolve(dir + string(os.PathSeparator) + ".")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if got != filepath.Clean(dir) {
		t.Fatalf("expected cleaned path %q, got %q", filepath.Clean(dir), got)
	}
}

func TestActiveSession_MissingFileReturnsErrNoActiveSession(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadActiveSession(dir); !errors.Is(err, ErrNoActiveSession) {
		t.Fatalf("expected ErrNoActiveSession, got %v", err)
	}
}

func TestWriteAndReadActiveSession_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	got, err := WriteActiveSession(dir, "  sess-1  ")
	if err != nil {
		t.Fatalf("WriteActiveSession: %v", err)
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("session ID not trimmed: %q", got.SessionID)
	}

	read, err := ReadActiveSession(dir)
	if err != nil {
		t.Fatalf("ReadActiveSession: %v", err)
	}
	if read.SessionID != "sess-1" {
		t.Fatalf("read back wrong ID: %q", read.SessionID)
	}
}

func TestWriteActiveSession_EmptyIDRejected(t *testing.T) {
	if _, err := WriteActiveSession(t.TempDir(), "   "); err == nil {
		t.Fatalf("expected error for empty session ID")
	}
}

func TestReadActiveSession_EmptyIDReturnsErrNoActiveSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ptolemy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SessionFilePath(dir), []byte(`{"session_id":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadActiveSession(dir); !errors.Is(err, ErrNoActiveSession) {
		t.Fatalf("expected ErrNoActiveSession for blank ID, got %v", err)
	}
}

func TestReadActiveSession_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ptolemy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SessionFilePath(dir), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadActiveSession(dir); err == nil {
		t.Fatalf("expected JSON error")
	}
}
