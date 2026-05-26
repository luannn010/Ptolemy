package store

import (
	"path/filepath"
	"testing"
)

func TestSQLDB_ReturnsUnderlyingHandle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if s.SQLDB() == nil {
		t.Fatalf("SQLDB returned nil")
	}
	if s.SQLDB() != s.DB {
		t.Fatalf("SQLDB must return the same handle as DB field")
	}
}

func TestOpen_BadPathReturnsError(t *testing.T) {
	// A path that points into a nonexistent directory triggers the pragma /
	// migration phase to fail. modernc.org/sqlite tolerates many odd paths,
	// so use a directory-as-file collision which it cannot open.
	dir := t.TempDir()
	bad := filepath.Join(dir, "subdir", "missing", "no.db")
	if _, err := Open(bad); err == nil {
		t.Fatalf("expected error for unreachable path %q", bad)
	}
}
