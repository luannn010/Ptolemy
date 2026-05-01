package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/luannn010/ptolemy/internal/approval"
)

func TestWorkerdBootRouter(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer baseStore.Close()

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	actionStore := action.NewStore(baseStore.SQLDB())
	logStore := logging.NewStore(baseStore.SQLDB())
	runner := terminal.NewTmuxRunner()

	approvalStore := approval.NewStore(baseStore.SQLDB())

	router := httpapi.NewRouter(sessionStore, commandStore, actionStore, logStore, approvalStore, runner)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
