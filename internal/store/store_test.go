package store

import (
	"testing"
)

func TestOpenStoreAndMigrate(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("expected no error opening store, got %v", err)
	}
	defer s.Close()

	if s.DB == nil {
		t.Fatal("expected DB to be initialized")
	}

	tables := []string{
		"sessions",
		"command_logs",
	}

	for _, table := range tables {
		var name string
		err := s.DB.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)

		if err != nil {
			t.Fatalf("expected table %s to exist: %v", table, err)
		}

		if name != table {
			t.Fatalf("expected table %s, got %s", table, name)
		}
	}
}

func TestStoreClose(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("expected no error opening store, got %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("expected close to succeed, got %v", err)
	}
}
