package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/policy"
)

type stubBrain struct {
	wake, stop, switched, status int
	lastModel                    string
	lastToken                    string
	statusOut                    brain.Status
	wakeErr, stopErr             error
	switchErr, statusErr         error
}

func (s *stubBrain) Wake(_ context.Context, _, model string, opts policy.CallOpts) error {
	s.wake++
	s.lastModel = model
	s.lastToken = opts.ConfirmToken
	return s.wakeErr
}
func (s *stubBrain) Stop(_ context.Context, _ string, _ policy.CallOpts) error {
	s.stop++
	return s.stopErr
}
func (s *stubBrain) Switch(_ context.Context, _, model string, _ policy.CallOpts) error {
	s.switched++
	s.lastModel = model
	return s.switchErr
}
func (s *stubBrain) Status(_ context.Context, _ string, _ policy.CallOpts) (brain.Status, error) {
	s.status++
	return s.statusOut, s.statusErr
}

func brainServer(t *testing.T, sb *stubBrain) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewBrainControlRouter(BrainDeps{Brain: sb}))
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, path, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestBrainStatus_OK(t *testing.T) {
	sb := &stubBrain{statusOut: brain.Status{Running: true, Model: "qwen9b"}}
	srv := brainServer(t, sb)
	resp, err := http.Get(srv.URL + "/brain/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var out brain.Status
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !out.Running || out.Model != "qwen9b" {
		t.Fatalf("unexpected status payload: %+v", out)
	}
}

func TestBrainWake_OK_PassesModelAndToken(t *testing.T) {
	sb := &stubBrain{}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/wake", `{"model":"qwen9b","confirm_token":"tok"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wake status %d", resp.StatusCode)
	}
	if sb.wake != 1 || sb.lastModel != "qwen9b" || sb.lastToken != "tok" {
		t.Fatalf("wake not wired through: %+v", sb)
	}
}

func TestBrainSwitch_NeedsConfirmation_202(t *testing.T) {
	sb := &stubBrain{switchErr: policy.ErrNeedsConfirmation{PendingID: "abc123", Channel: "oob", Reason: "manual swap"}}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/switch", `{"model":"qwen4b"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var out struct {
		Status    string `json:"status"`
		PendingID string `json:"pending_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Status != "needs_confirmation" || out.PendingID != "abc123" {
		t.Fatalf("unexpected needs-confirmation payload: %+v", out)
	}
}

func TestBrainStop_Denied_403(t *testing.T) {
	sb := &stubBrain{stopErr: policy.ErrDenied{RuleID: "deny-x", Reason: "nope"}}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/stop", `{}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestBrainSwitch_MissingModel_400(t *testing.T) {
	sb := &stubBrain{}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/switch", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing model, got %d", resp.StatusCode)
	}
	if sb.switched != 0 {
		t.Fatal("switch must not run without a model")
	}
}

func TestBrain_NonLoopbackForbidden(t *testing.T) {
	h := NewBrainControlRouter(BrainDeps{Brain: &stubBrain{}})
	req := httptest.NewRequest(http.MethodGet, "/brain/status", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback must be forbidden, got %d", rec.Code)
	}
}
