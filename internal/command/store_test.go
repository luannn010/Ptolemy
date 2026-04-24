package command

import (
	"context"
	"testing"

	"github.com/luannn010/ptolemy/internal/store"
)

func newTestCommandStore(t *testing.T) *Store {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"

	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}

	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	return NewStore(baseStore)
}

func TestCreateCommandLog(t *testing.T) {
	commandStore := newTestCommandStore(t)

	logItem, err := commandStore.Create(context.Background(), CommandLog{
		SessionID:   "test-session",
		Command:     "echo hello",
		CWD:         "/tmp",
		ExitCode:    0,
		Output:      "hello\n",
		ErrorOutput: "",
		DurationMS:  10,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if logItem.ID == "" {
		t.Fatal("expected command log ID")
	}

	if logItem.Command != "echo hello" {
		t.Fatalf("expected command echo hello, got %s", logItem.Command)
	}
}

func TestListCommandLogsBySession(t *testing.T) {
	commandStore := newTestCommandStore(t)

	_, err := commandStore.Create(context.Background(), CommandLog{
		SessionID:  "session-1",
		Command:    "echo one",
		CWD:        "/tmp",
		ExitCode:   0,
		Output:     "one\n",
		DurationMS: 5,
	})
	if err != nil {
		t.Fatalf("create first log failed: %v", err)
	}

	_, err = commandStore.Create(context.Background(), CommandLog{
		SessionID:  "session-1",
		Command:    "echo two",
		CWD:        "/tmp",
		ExitCode:   0,
		Output:     "two\n",
		DurationMS: 5,
	})
	if err != nil {
		t.Fatalf("create second log failed: %v", err)
	}

	logs, err := commandStore.ListBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("list logs failed: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
}
