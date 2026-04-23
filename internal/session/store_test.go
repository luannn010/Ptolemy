package session

import (
	"context"
	"testing"

	"github.com/luannn010/ptolemy/internal/store"
)

func newTestSessionStore(t *testing.T) *Store {
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

func TestCreateSession(t *testing.T) {
	sessionStore := newTestSessionStore(t)

	sess, err := sessionStore.Create(context.Background(), CreateSessionRequest{
		Name:        "test-session",
		Workspace:   "/tmp/project",
		Description: "test description",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if sess.ID == "" {
		t.Fatal("expected session ID to be generated")
	}

	if sess.Name != "test-session" {
		t.Fatalf("expected name test-session, got %s", sess.Name)
	}

	if sess.Status != StatusOpen {
		t.Fatalf("expected status open, got %s", sess.Status)
	}
}

func TestGetSession(t *testing.T) {
	sessionStore := newTestSessionStore(t)

	created, err := sessionStore.Create(context.Background(), CreateSessionRequest{
		Name:      "get-session",
		Workspace: "/tmp/project",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	found, err := sessionStore.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if found.ID != created.ID {
		t.Fatalf("expected id %s, got %s", created.ID, found.ID)
	}
}

func TestListSessions(t *testing.T) {
	sessionStore := newTestSessionStore(t)

	_, err := sessionStore.Create(context.Background(), CreateSessionRequest{
		Name:      "session-one",
		Workspace: "/tmp/project-one",
	})
	if err != nil {
		t.Fatalf("create session one failed: %v", err)
	}

	_, err = sessionStore.Create(context.Background(), CreateSessionRequest{
		Name:      "session-two",
		Workspace: "/tmp/project-two",
	})
	if err != nil {
		t.Fatalf("create session two failed: %v", err)
	}

	sessions, err := sessionStore.List(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestCloseSession(t *testing.T) {
	sessionStore := newTestSessionStore(t)

	created, err := sessionStore.Create(context.Background(), CreateSessionRequest{
		Name:      "close-session",
		Workspace: "/tmp/project",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	closed, err := sessionStore.CloseSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}

	if closed.Status != StatusClosed {
		t.Fatalf("expected status closed, got %s", closed.Status)
	}

	if closed.ClosedAt == nil {
		t.Fatal("expected closed_at to be set")
	}
}

func TestSessionPersistsAfterStoreReopen(t *testing.T) {
	dbPath := t.TempDir() + "/persist.db"

	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open first store: %v", err)
	}

	sessionStore := NewStore(baseStore)

	created, err := sessionStore.Create(context.Background(), CreateSessionRequest{
		Name:      "persist-session",
		Workspace: "/tmp/project",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := baseStore.Close(); err != nil {
		t.Fatalf("close first store failed: %v", err)
	}

	reopenedBaseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	defer reopenedBaseStore.Close()

	reopenedSessionStore := NewStore(reopenedBaseStore)

	found, err := reopenedSessionStore.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected session to persist after reopen, got error: %v", err)
	}

	if found.ID != created.ID {
		t.Fatalf("expected id %s, got %s", created.ID, found.ID)
	}
}

func TestGetMissingSession(t *testing.T) {
	sessionStore := newTestSessionStore(t)

	_, err := sessionStore.Get(context.Background(), "missing-id")
	if err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}