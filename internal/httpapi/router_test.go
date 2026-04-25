package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"

	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	runner := terminal.NewTmuxRunner()

	return NewRouter(sessionStore, commandStore, runner)
}

func TestHealthEndpoint(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected health ok response, got %s", rec.Body.String())
	}
}

func TestCreateSessionEndpoint(t *testing.T) {
	router := newTestRouter(t)

	body := strings.NewReader(`{
		"name": "http-test-session",
		"workspace": "/tmp",
		"description": "created from http test"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), `"name":"http-test-session"`) {
		t.Fatalf("expected created session response, got %s", rec.Body.String())
	}
}

func TestListSessionsEndpoint(t *testing.T) {
	router := newTestRouter(t)

	createBody := strings.NewReader(`{
		"name": "list-session",
		"workspace": "/tmp"
	}`)

	createReq := httptest.NewRequest(http.MethodPost, "/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()

	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d", createRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	listRec := httptest.NewRecorder()

	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listRec.Code)
	}

	if !strings.Contains(listRec.Body.String(), `"name":"list-session"`) {
		t.Fatalf("expected list response to include session, got %s", listRec.Body.String())
	}
}

func TestFileWriteAndReadEndpoints(t *testing.T) {
	router := newTestRouter(t)
	workspace := t.TempDir()

	createBody := strings.NewReader(`{
		"name": "file-session",
		"workspace": "` + workspace + `"
	}`)

	createReq := httptest.NewRequest(http.MethodPost, "/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()

	router.ServeHTTP(createRec, createReq)

	sessionID := extractJSONField(t, createRec.Body.String(), "id")

	writeBody := strings.NewReader(`{
		"session_id": "` + sessionID + `",
		"path": "hello.txt",
		"content": "hello api"
	}`)

	writeReq := httptest.NewRequest(http.MethodPost, "/file/write", writeBody)
	writeReq.Header.Set("Content-Type", "application/json")
	writeRec := httptest.NewRecorder()

	router.ServeHTTP(writeRec, writeReq)

	if writeRec.Code != http.StatusOK {
		t.Fatalf("expected write status 200, got %d body=%s", writeRec.Code, writeRec.Body.String())
	}

	readBody := strings.NewReader(`{
		"session_id": "` + sessionID + `",
		"path": "hello.txt"
	}`)

	readReq := httptest.NewRequest(http.MethodPost, "/file/read", readBody)
	readReq.Header.Set("Content-Type", "application/json")
	readRec := httptest.NewRecorder()

	router.ServeHTTP(readRec, readReq)

	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	if !strings.Contains(readRec.Body.String(), "hello api") {
		t.Fatalf("expected read content, got %s", readRec.Body.String())
	}
}

func extractJSONField(t *testing.T, body string, field string) string {
	t.Helper()

	needle := `"` + field + `":"`
	start := strings.Index(body, needle)
	if start == -1 {
		t.Fatalf("field %s not found in body: %s", field, body)
	}

	start += len(needle)
	end := strings.Index(body[start:], `"`)
	if end == -1 {
		t.Fatalf("field %s not closed in body: %s", field, body)
	}

	return body[start : start+end]
}
