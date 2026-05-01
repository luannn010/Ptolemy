package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/logs"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/skills"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
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
	logStore := logs.NewStore(baseStore.SQLDB())
	runner := terminal.NewTmuxRunner()

	approvalStore := approval.NewStore(baseStore.SQLDB())
	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, ".ptolemy", "server", "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	skillRegistry, err := skills.NewRegistry(baseDir, skillDir)
	if err != nil {
		t.Fatalf("failed to create skill registry: %v", err)
	}

	router := httpapi.NewRouter(sessionStore, commandStore, actionStore, logStore, approvalStore, runner, skillRegistry)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
