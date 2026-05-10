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

func TestOpenStoreConfiguresSQLiteForWorkerConcurrency(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("expected no error opening store, got %v", err)
	}
	defer s.Close()

	if got := s.DB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("expected MaxOpenConnections to be 1, got %d", got)
	}

	var busyTimeout int
	if err := s.DB.QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("expected busy_timeout pragma to be readable, got %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("expected busy_timeout to be 5000, got %d", busyTimeout)
	}

	var journalMode string
	if err := s.DB.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("expected journal_mode pragma to be readable, got %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected journal_mode to be wal, got %s", journalMode)
	}
}
