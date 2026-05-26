package store

import (
	"fmt"
	"sort"
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

	expectedTables := []string{
		"command_logs",
		"policy_decisions",
		"schema_migrations",
		"sessions",
	}

	rows, err := s.DB.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("expected to list tables, got %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("expected row scan to succeed, got %v", err)
		}
		if name == "sqlite_sequence" {
			continue
		}
		tables = append(tables, name)
	}
	sort.Strings(tables)

	if fmt.Sprintf("%v", tables) != fmt.Sprintf("%v", expectedTables) {
		t.Fatalf("expected exactly tables %v, got %v", expectedTables, tables)
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

func TestOpenStoreIsIdempotentAcrossReopen(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("expected first open to succeed, got %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("expected first close to succeed, got %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("expected second open to succeed, got %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version='v1'").Scan(&count); err != nil {
		t.Fatalf("expected schema_migrations query to succeed, got %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migration v1 recorded once, got %d", count)
	}
}
