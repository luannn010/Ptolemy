package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/logging"
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

	if err := store.RunMigrations(t.Context(), baseStore.SQLDB()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	actionStore := action.NewStore(baseStore.SQLDB())
	logStore := logging.NewStore(baseStore.SQLDB())
	approvalStore := approval.NewStore(baseStore.SQLDB())
	runner := terminal.NewTmuxRunner()

	return NewRouter(sessionStore, commandStore, actionStore, logStore, approvalStore, runner)
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

func TestExecuteEndpointAcceptsDescriptiveMetadata(t *testing.T) {
	router := newTestRouter(t)

	createBody := strings.NewReader(`{
		"name": "execute-session",
		"workspace": "/tmp"
	}`)

	createReq := httptest.NewRequest(http.MethodPost, "/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()

	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	sessionID := extractJSONField(t, createRec.Body.String(), "id")

	executeBody := strings.NewReader(`{
		"session_id": "` + sessionID + `",
		"command": "echo execute-metadata-ok",
		"timeout": 5,
		"title": "Run execute metadata test",
		"purpose": "Confirm /execute accepts descriptive metadata.",
		"reasoning_step": "Validate executor endpoint",
		"target": "echo execute-metadata-ok"
	}`)

	executeReq := httptest.NewRequest(http.MethodPost, "/execute", executeBody)
	executeReq.Header.Set("Content-Type", "application/json")
	executeRec := httptest.NewRecorder()

	router.ServeHTTP(executeRec, executeReq)

	if executeRec.Code != http.StatusOK {
		t.Fatalf("expected execute status 200, got %d body=%s", executeRec.Code, executeRec.Body.String())
	}
	if !strings.Contains(executeRec.Body.String(), "execute-metadata-ok") {
		t.Fatalf("expected execute response output, got %s", executeRec.Body.String())
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

func TestNavigatorEndpointsAndFileReadTracking(t *testing.T) {
	router := newTestRouter(t)
	workspace := t.TempDir()

	createBody := strings.NewReader(`{
		"name": "navigator-session",
		"workspace": "` + workspace + `"
	}`)

	createReq := httptest.NewRequest(http.MethodPost, "/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()

	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	sessionID := extractJSONField(t, createRec.Body.String(), "id")

	indexBody := strings.NewReader(`{"session_id":"` + sessionID + `"}`)
	indexReq := httptest.NewRequest(http.MethodPost, "/navigator/index", indexBody)
	indexReq.Header.Set("Content-Type", "application/json")
	indexRec := httptest.NewRecorder()

	router.ServeHTTP(indexRec, indexReq)

	if indexRec.Code != http.StatusOK {
		t.Fatalf("expected index status 200, got %d body=%s", indexRec.Code, indexRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, ".ptolemy", "index", "file-tree.json")); err != nil {
		t.Fatalf("expected file-tree index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".ptolemy", "kb", "FILE_INDEX.json")); err != nil {
		t.Fatalf("expected KB file index: %v", err)
	}

	kbReadBody := strings.NewReader(`{"session_id":"` + sessionID + `"}`)
	kbReadReq := httptest.NewRequest(http.MethodPost, "/kb/read", kbReadBody)
	kbReadReq.Header.Set("Content-Type", "application/json")
	kbReadRec := httptest.NewRecorder()

	router.ServeHTTP(kbReadRec, kbReadReq)

	if kbReadRec.Code != http.StatusOK {
		t.Fatalf("expected kb read status 200, got %d body=%s", kbReadRec.Code, kbReadRec.Body.String())
	}
	if !strings.Contains(kbReadRec.Body.String(), ".ptolemy/kb/PROJECT_MAP.md") {
		t.Fatalf("expected kb read response to include project map, got %s", kbReadRec.Body.String())
	}

	taskBody := strings.NewReader(`{
		"session_id": "` + sessionID + `",
		"task_session_id": "navigator-test",
		"task": "Track files read"
	}`)
	taskReq := httptest.NewRequest(http.MethodPost, "/navigator/session/start", taskBody)
	taskReq.Header.Set("Content-Type", "application/json")
	taskRec := httptest.NewRecorder()

	router.ServeHTTP(taskRec, taskReq)

	if taskRec.Code != http.StatusOK {
		t.Fatalf("expected task session status 200, got %d body=%s", taskRec.Code, taskRec.Body.String())
	}

	writeBody := strings.NewReader(`{
		"session_id": "` + sessionID + `",
		"path": "hello.txt",
		"content": "hello navigator"
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
		"task_session_id": "navigator-test",
		"path": "hello.txt"
	}`)
	readReq := httptest.NewRequest(http.MethodPost, "/file/read", readBody)
	readReq.Header.Set("Content-Type", "application/json")
	readRec := httptest.NewRecorder()

	router.ServeHTTP(readRec, readReq)

	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	filesRead, err := os.ReadFile(filepath.Join(workspace, ".ptolemy", "sessions", "navigator-test", "files-read.json"))
	if err != nil {
		t.Fatalf("expected files-read log: %v", err)
	}
	if !strings.Contains(string(filesRead), "hello.txt") {
		t.Fatalf("expected files-read to contain hello.txt, got %s", string(filesRead))
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
