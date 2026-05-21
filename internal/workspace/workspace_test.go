package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveReturnsAbsoluteCleanDirectory(t *testing.T) {
	dir := t.TempDir()

	got, err := Resolve(filepath.Join(dir, "."))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("Resolve() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestReadActiveSessionMissing(t *testing.T) {
	_, err := ReadActiveSession(t.TempDir())
	if !errors.Is(err, ErrNoActiveSession) {
		t.Fatalf("ReadActiveSession() error = %v, want ErrNoActiveSession", err)
	}
}

func TestWriteReadActiveSession(t *testing.T) {
	dir := t.TempDir()

	written, err := WriteActiveSession(dir, "session-123")
	if err != nil {
		t.Fatalf("WriteActiveSession() error = %v", err)
	}
	if written.SessionID != "session-123" {
		t.Fatalf("SessionID = %q, want session-123", written.SessionID)
	}

	read, err := ReadActiveSession(dir)
	if err != nil {
		t.Fatalf("ReadActiveSession() error = %v", err)
	}
	if read.SessionID != "session-123" {
		t.Fatalf("SessionID = %q, want session-123", read.SessionID)
	}

	if _, err := os.Stat(filepath.Join(dir, ".ptolemy", "session.json")); err != nil {
		t.Fatalf("expected session file: %v", err)
	}
}
