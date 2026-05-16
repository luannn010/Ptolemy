package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/agentloop"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/packstudio"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

func newPackStudioTestRouter(t *testing.T) http.Handler {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "pack-studio.db")

	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	if err := store.RunMigrations(t.Context(), baseStore.SQLDB()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	service := packstudio.NewService(
		root,
		packstudio.NewStore(baseStore.SQLDB()),
		agentloop.NewStore(baseStore.SQLDB()),
		nil,
		action.NewStore(baseStore.SQLDB()),
		command.NewStore(baseStore),
		terminal.NewTmuxRunner(),
	)

	return NewPackStudioHandler(service).Routes()
}

func TestPackStudioShellServesIndex(t *testing.T) {
	router := newPackStudioTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/ui/assets/app.js") {
		t.Fatalf("expected shell to include app bundle, got %s", rec.Body.String())
	}
}

func TestPackStudioCreatePackAndListIt(t *testing.T) {
	router := newPackStudioTestRouter(t)

	body, err := json.Marshal(packstudio.CreatePackInput{
		PackID:            "ui-pack",
		Name:              "UI Pack",
		Description:       "Pack created through the HTTP handler.",
		Goal:              "Verify pack authoring endpoints.",
		CreatedBy:         "tests",
		MaxAllowedFiles:   3,
		RequireValidation: true,
		RequireBranch:     true,
		StopOnFailure:     true,
		Tasks: []packstudio.PackTaskInput{
			{
				ID:           "build-overview",
				Title:        "Build overview",
				Summary:      "Create the overview page.",
				AllowedFiles: []string{"internal/httpapi"},
				Validation:   []string{"go test ./internal/httpapi"},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/packs", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/packs", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "\"id\":\"ui-pack\"") {
		t.Fatalf("expected pack catalog to include ui-pack, got %s", listRec.Body.String())
	}

	planReq := httptest.NewRequest(http.MethodGet, "/api/packs/ui-pack/plan", nil)
	planRec := httptest.NewRecorder()
	router.ServeHTTP(planRec, planReq)

	if planRec.Code != http.StatusOK {
		t.Fatalf("expected plan status 200, got %d body=%s", planRec.Code, planRec.Body.String())
	}
	if !strings.Contains(planRec.Body.String(), "\"build-overview\"") {
		t.Fatalf("expected plan response to include task id, got %s", planRec.Body.String())
	}
}
