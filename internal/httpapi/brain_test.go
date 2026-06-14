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
	loaded                              *brain.Spec
	lastToken                           string
	resume, hibernate, stop, listModels int
	statusOut                           brain.Status
	loadErr, resumeErr, stopErr         error
	statusErr, listErr                  error
}

func (s *stubBrain) Load(_ context.Context, _ string, spec brain.Spec, opts policy.CallOpts) error {
	s.loaded = &spec
	s.lastToken = opts.ConfirmToken
	return s.loadErr
}
func (s *stubBrain) Resume(_ context.Context, _ string, _ policy.CallOpts) error {
	s.resume++
	return s.resumeErr
}
func (s *stubBrain) Hibernate(_ context.Context, _ string, _ policy.CallOpts) error {
	s.hibernate++
	return nil
}
func (s *stubBrain) Stop(_ context.Context, _ string, _ policy.CallOpts) error {
	s.stop++
	return s.stopErr
}
func (s *stubBrain) Status(_ context.Context, _ string, _ policy.CallOpts) (brain.Status, error) {
	return s.statusOut, s.statusErr
}
func (s *stubBrain) ListModels(_ context.Context, _ string, _ policy.CallOpts) ([]brain.DiscoveredModel, error) {
	s.listModels++
	return []brain.DiscoveredModel{{Name: "m.gguf", Path: "/m.gguf", Size: 1}}, s.listErr
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
	sb := &stubBrain{statusOut: brain.Status{Running: true, GGUF: "/m.gguf"}}
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
	if !out.Running || out.GGUF != "/m.gguf" {
		t.Fatalf("unexpected status payload: %+v", out)
	}
}

func TestBrainModels_OK(t *testing.T) {
	sb := &stubBrain{}
	srv := brainServer(t, sb)
	resp, err := http.Get(srv.URL + "/brain/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("models status %d", resp.StatusCode)
	}
	if sb.listModels != 1 {
		t.Fatalf("ListModels not called: %+v", sb)
	}
}

func TestBrainLoad_OK_PassesSpecAndToken(t *testing.T) {
	sb := &stubBrain{}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/load",
		`{"binary":"/b","gguf":"/m.gguf","host":"0.0.0.0","port":"9000","args":["-ngl","999"],"confirm_token":"tok"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("load status %d", resp.StatusCode)
	}
	if sb.loaded == nil || sb.loaded.GGUF != "/m.gguf" || sb.loaded.Binary != "/b" ||
		len(sb.loaded.Args) != 2 || sb.lastToken != "tok" {
		t.Fatalf("load not wired through: %+v", sb.loaded)
	}
}

func TestBrainLoad_MissingGGUF_400(t *testing.T) {
	sb := &stubBrain{}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/load", `{"binary":"/b"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing gguf must be 400, got %d", resp.StatusCode)
	}
	if sb.loaded != nil {
		t.Fatal("load must not run without a gguf")
	}
}

func TestBrainLoad_NeedsConfirmation_202(t *testing.T) {
	sb := &stubBrain{loadErr: policy.ErrNeedsConfirmation{PendingID: "abc123", Channel: "oob", Reason: "custom launch"}}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/load", `{"gguf":"/m.gguf"}`)
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

func TestBrainResume_NoModel_409(t *testing.T) {
	sb := &stubBrain{resumeErr: brain.ErrNoModelLoaded}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/resume", `{}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("no-model resume must be 409, got %d", resp.StatusCode)
	}
}

func TestBrainHibernate_OK(t *testing.T) {
	sb := &stubBrain{}
	srv := brainServer(t, sb)
	resp := post(t, srv, "/brain/hibernate", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hibernate status %d", resp.StatusCode)
	}
	if sb.hibernate != 1 {
		t.Fatalf("hibernate not called: %+v", sb)
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
