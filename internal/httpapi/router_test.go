package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/apitypes"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/health"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type stubRunner struct {
	res terminal.Result
	err error
}

func (s stubRunner) Run(_ context.Context, _ string, _ string, _ string, _ int, _ policy.CallOpts) (terminal.Result, error) {
	return s.res, s.err
}

func newTestRouter(t *testing.T, runner command.GuardedRunner) (http.Handler, *session.Store) {
	t.Helper()
	base, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })

	sessions := session.NewStore(base)
	cmdStore := command.NewStore(base)
	commands := command.NewService(runner, cmdStore)
	return NewRouter(RouterDeps{Sessions: sessions, Commands: commands, CommandDB: cmdStore}), sessions
}

func TestHealthEndpoint(t *testing.T) {
	h, _ := newTestRouter(t, stubRunner{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %+v", body)
	}
}

func TestHealth_DeepOK(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	agg := &health.Aggregator{Checkers: []health.Checker{
		health.NewHTTPChecker("brain", up.URL, "/v1/models", true),
		health.NewHTTPChecker("embedder", up.URL, "/v1/models", true),
		health.NewPgChecker("postgres", nil),               // disabled
		health.NewHTTPChecker("mcp", "", "/health", false), // disabled
	}}
	router := NewRouter(RouterDeps{Health: agg})

	srv := httptest.NewServer(router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code = %d, want 200", resp.StatusCode)
	}
	var report health.Report
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "ok" || report.Service != "workerd" || len(report.Checks) != 4 {
		t.Fatalf("report = %+v", report)
	}
}

func TestHealth_RequiredDown503(t *testing.T) {
	agg := &health.Aggregator{Checkers: []health.Checker{
		health.NewHTTPChecker("brain", "", "/v1/models", true), // required, not configured -> down
	}}
	router := NewRouter(RouterDeps{Health: agg})

	srv := httptest.NewServer(router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", resp.StatusCode)
	}
}

func TestHealth_NilFallback(t *testing.T) {
	router := NewRouter(RouterDeps{}) // no Health wired
	srv := httptest.NewServer(router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code = %d, want 200", resp.StatusCode)
	}
}

func TestCreateAndListSessions(t *testing.T) {
	h, _ := newTestRouter(t, stubRunner{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(apitypes.CreateSessionRequest{Name: "n", Workspace: ".", Description: "d"})
	resp, err := http.Post(srv.URL+"/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status %d", resp.StatusCode)
	}
	var created apitypes.SessionResponse
	_ = json.NewDecoder(resp.Body).Decode(&created)
	if created.ID == "" {
		t.Fatalf("expected ID")
	}

	resp, err = http.Get(srv.URL + "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	var list apitypes.SessionListResponse
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list.Sessions))
	}

	resp, err = http.Get(srv.URL + "/sessions/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get one status %d", resp.StatusCode)
	}
}

func TestCreateSession_InvalidJSON(t *testing.T) {
	h, _ := newTestRouter(t, stubRunner{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, _ := http.Post(srv.URL+"/sessions", "application/json", bytes.NewReader([]byte("not json")))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetMissingSession_Returns404(t *testing.T) {
	h, _ := newTestRouter(t, stubRunner{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/sessions/does-not-exist")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRunCommand_Allow(t *testing.T) {
	h, sessions := newTestRouter(t, stubRunner{res: terminal.Result{ExitCode: 0, Output: "ok"}})
	srv := httptest.NewServer(h)
	defer srv.Close()

	s, err := sessions.Create(context.Background(), session.CreateSessionRequest{Name: "s", Workspace: ".", Description: ""})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(apitypes.RunCommandRequest{Command: "go test", CWD: ".", Timeout: 5})
	resp, _ := http.Post(srv.URL+"/sessions/"+s.ID+"/commands", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var log command.CommandLog
	_ = json.NewDecoder(resp.Body).Decode(&log)
	if log.Output != "ok" {
		t.Fatalf("unexpected output %q", log.Output)
	}

	resp, _ = http.Get(srv.URL + "/sessions/" + s.ID + "/commands")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status %d", resp.StatusCode)
	}
}

func TestRunCommand_Denied(t *testing.T) {
	h, sessions := newTestRouter(t, stubRunner{err: policy.ErrDenied{RuleID: "deny-x", Reason: "no"}})
	srv := httptest.NewServer(h)
	defer srv.Close()
	s, _ := sessions.Create(context.Background(), session.CreateSessionRequest{Name: "s"})

	body, _ := json.Marshal(apitypes.RunCommandRequest{Command: "rm -rf /"})
	resp, _ := http.Post(srv.URL+"/sessions/"+s.ID+"/commands", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRunCommand_NeedsConfirmation(t *testing.T) {
	h, sessions := newTestRouter(t, stubRunner{err: policy.ErrNeedsConfirmation{PendingID: "pid", Channel: domain.ChannelOOB, Reason: "ask"}})
	srv := httptest.NewServer(h)
	defer srv.Close()
	s, _ := sessions.Create(context.Background(), session.CreateSessionRequest{Name: "s"})

	body, _ := json.Marshal(apitypes.RunCommandRequest{Command: "git push"})
	resp, _ := http.Post(srv.URL+"/sessions/"+s.ID+"/commands", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var nc apitypes.NeedsConfirmation
	_ = json.NewDecoder(resp.Body).Decode(&nc)
	if nc.PendingID != "pid" {
		t.Fatalf("expected pid, got %+v", nc)
	}
}

func TestExecuteEndpoint(t *testing.T) {
	h, sessions := newTestRouter(t, stubRunner{res: terminal.Result{ExitCode: 0}})
	srv := httptest.NewServer(h)
	defer srv.Close()
	s, _ := sessions.Create(context.Background(), session.CreateSessionRequest{Name: "s"})

	payload := map[string]any{
		"session_id":    s.ID,
		"command":       "echo hi",
		"confirm_token": "tok",
	}
	body, _ := json.Marshal(payload)
	resp, _ := http.Post(srv.URL+"/execute", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCloseSession(t *testing.T) {
	h, sessions := newTestRouter(t, stubRunner{})
	srv := httptest.NewServer(h)
	defer srv.Close()
	s, _ := sessions.Create(context.Background(), session.CreateSessionRequest{Name: "s"})

	resp, _ := http.Post(srv.URL+"/sessions/"+s.ID+"/close", "application/json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestApproveRouter_RejectsNonLoopback(t *testing.T) {
	approvals := policy.NewApprovals()
	approvals.Park("pid1")
	h := NewApproveRouter(approvals)

	req := httptest.NewRequest(http.MethodPost, "/approve/pid1", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback must be forbidden, got %d", rec.Code)
	}
}

func TestApproveRouter_ApprovesLoopback(t *testing.T) {
	approvals := policy.NewApprovals()
	approvals.Park("pid2")
	h := NewApproveRouter(approvals)

	req := httptest.NewRequest(http.MethodPost, "/approve/pid2", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback approve should succeed, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !approvals.ConsumeApproved("pid2") {
		t.Fatalf("expected approval to be consumable after Approve")
	}
}

// TestAsDenied_* tests the unexported helper directly — package httpapi has access.
func TestAsDenied_NilError(t *testing.T) {
	var denied policy.ErrDenied
	if asDenied(nil, &denied) {
		t.Fatal("asDenied(nil) must return false")
	}
}

func TestAsDenied_NonErrDenied(t *testing.T) {
	var denied policy.ErrDenied
	if asDenied(fmt.Errorf("plain error"), &denied) {
		t.Fatal("asDenied with a plain error must return false")
	}
}

func TestRunCommand_GeneralError_Returns500(t *testing.T) {
	h, sessions := newTestRouter(t, stubRunner{err: fmt.Errorf("general boom")})
	srv := httptest.NewServer(h)
	defer srv.Close()
	s, _ := sessions.Create(context.Background(), session.CreateSessionRequest{Name: "s"})

	body, _ := json.Marshal(apitypes.RunCommandRequest{Command: "echo hi"})
	resp, _ := http.Post(srv.URL+"/sessions/"+s.ID+"/commands", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 for general error, got %d", resp.StatusCode)
	}
}

func TestApproveRouter_UnknownPendingID(t *testing.T) {
	h := NewApproveRouter(policy.NewApprovals())
	req := httptest.NewRequest(http.MethodPost, "/approve/never-parked", nil)
	req.RemoteAddr = "127.0.0.1:1111"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
