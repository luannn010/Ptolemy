package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luannn010/ptolemy/internal/apitypes"
	"github.com/luannn010/ptolemy/internal/memory"
)

// stubAnswerer captures the Query it was called with and returns a canned Answer.
type stubAnswerer struct {
	mu    sync.Mutex
	got   memory.Query
	calls int
	out   memory.Answer
	err   error
}

func (s *stubAnswerer) Answer(_ context.Context, q memory.Query) (memory.Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = q
	s.calls++
	return s.out, s.err
}

func newRAGServer(t *testing.T, stub *stubAnswerer) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewRAGRouter(RAGDeps{
		Answerer:       stub,
		DefaultSubject: "default",
		DefaultProject: "ptolemy",
	}))
	t.Cleanup(srv.Close)
	return srv
}

func postChat(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+"/chat", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeChat(t *testing.T, resp *http.Response) apitypes.ChatResponse {
	t.Helper()
	defer resp.Body.Close()
	var out apitypes.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestChat_HappyPath(t *testing.T) {
	stub := &stubAnswerer{out: memory.Answer{Text: "the answer [source:a]", Citations: []string{"a"}}}
	srv := newRAGServer(t, stub)

	resp := postChat(t, srv, `{"query":"q"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	out := decodeChat(t, resp)
	if out.Answer != "the answer [source:a]" || len(out.Citations) != 1 || out.GaveUp {
		t.Fatalf("unexpected payload: %+v", out)
	}
	if stub.got.Text != "q" {
		t.Fatalf("query text not passed: %q", stub.got.Text)
	}
}

func TestChat_DefaultScope(t *testing.T) {
	stub := &stubAnswerer{}
	srv := newRAGServer(t, stub)

	postChat(t, srv, `{"query":"q"}`)
	if stub.got.SubjectID == nil || *stub.got.SubjectID != "default" {
		t.Fatalf("expected default subject, got %v", stub.got.SubjectID)
	}
	if stub.got.ProjectID == nil || *stub.got.ProjectID != "ptolemy" {
		t.Fatalf("expected default project, got %v", stub.got.ProjectID)
	}
}

func TestChat_ExplicitScopeOverrides(t *testing.T) {
	stub := &stubAnswerer{}
	srv := newRAGServer(t, stub)

	postChat(t, srv, `{"query":"q","subject_id":"alice","project_id":"proj2"}`)
	if stub.got.SubjectID == nil || *stub.got.SubjectID != "alice" {
		t.Fatalf("subject override lost: %v", stub.got.SubjectID)
	}
	if stub.got.ProjectID == nil || *stub.got.ProjectID != "proj2" {
		t.Fatalf("project override lost: %v", stub.got.ProjectID)
	}
}

func TestChat_KPassedThrough(t *testing.T) {
	stub := &stubAnswerer{}
	srv := newRAGServer(t, stub)

	postChat(t, srv, `{"query":"q","k":3}`)
	if stub.got.K != 3 {
		t.Fatalf("expected K=3, got %d", stub.got.K)
	}
	postChat(t, srv, `{"query":"q"}`)
	if stub.got.K != 0 {
		t.Fatalf("omitted k must stay 0 (orchestrator default), got %d", stub.got.K)
	}
}

func TestChat_InvalidJSON_400(t *testing.T) {
	srv := newRAGServer(t, &stubAnswerer{})
	resp := postChat(t, srv, `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestChat_EmptyQuery_400(t *testing.T) {
	stub := &stubAnswerer{}
	srv := newRAGServer(t, stub)
	for _, body := range []string{`{"query":""}`, `{"query":"   "}`, `{}`} {
		resp := postChat(t, srv, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400, got %d", body, resp.StatusCode)
		}
	}
	if stub.calls != 0 {
		t.Fatalf("answerer must not be called on bad input, calls=%d", stub.calls)
	}
}

func TestChat_TraceRequested(t *testing.T) {
	stub := &stubAnswerer{out: memory.Answer{
		Text:      "ans [source:a]",
		Citations: []string{"a"},
		Trace: &memory.RecallTrace{Mode: "agentic", Steps: []memory.TraceStep{
			{Index: 0, Action: "retrieve", Query: "terms",
				Retrieved: []memory.TraceChunk{{ID: "a", Score: 0.5, Snippet: "snip"}}},
			{Index: 1, Action: "answer", GroundingOK: true},
		}},
	}}
	srv := newRAGServer(t, stub)

	resp := postChat(t, srv, `{"query":"q","trace":true}`)
	out := decodeChat(t, resp)
	if !stub.got.Trace {
		t.Fatal("Query.Trace must be true")
	}
	if out.Mode != "agentic" || len(out.Steps) != 2 {
		t.Fatalf("trace not carried: %+v", out)
	}
	if out.Steps[0].Action != "retrieve" || len(out.Steps[0].Retrieved) != 1 || out.Steps[0].Retrieved[0].ID != "a" {
		t.Fatalf("step0 wrong: %+v", out.Steps[0])
	}
}

func TestChat_NoTrace_OmitsModeAndSteps(t *testing.T) {
	stub := &stubAnswerer{out: memory.Answer{Text: "ans", Citations: []string{"a"}}}
	srv := newRAGServer(t, stub)

	resp := postChat(t, srv, `{"query":"q"}`)
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Contains(s, `"mode"`) || strings.Contains(s, `"steps"`) {
		t.Fatalf("mode/steps must be omitted without trace, got %s", s)
	}
}

func TestChat_GaveUp_200(t *testing.T) {
	stub := &stubAnswerer{out: memory.Answer{Text: "I don't know based on what I found: no chunks", GaveUp: true}}
	srv := newRAGServer(t, stub)

	resp := postChat(t, srv, `{"query":"q"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gave_up must be 200, got %d", resp.StatusCode)
	}
	out := decodeChat(t, resp)
	if !out.GaveUp {
		t.Fatal("expected gave_up=true")
	}
}

func TestChat_AnswerError_502(t *testing.T) {
	stub := &stubAnswerer{err: errors.New("brain unreachable")}
	srv := newRAGServer(t, stub)

	resp := postChat(t, srv, `{"query":"q"}`)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("upstream failure must be 502, got %d", resp.StatusCode)
	}
}

func TestChat_NilCitations_SerializesEmptyArray(t *testing.T) {
	stub := &stubAnswerer{out: memory.Answer{Text: "ans", Citations: nil}}
	srv := newRAGServer(t, stub)

	resp := postChat(t, srv, `{"query":"q"}`)
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"citations":[]`) {
		t.Fatalf("nil citations must serialize as [], got %s", buf.String())
	}
}

func TestChat_MethodNotAllowed(t *testing.T) {
	srv := newRAGServer(t, &stubAnswerer{})
	resp, err := http.Get(srv.URL + "/chat")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /chat must be 405, got %d", resp.StatusCode)
	}
}

func TestRAGHealth_OK(t *testing.T) {
	srv := newRAGServer(t, &stubAnswerer{})
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status %d", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Fatalf("unexpected health payload: %v", out)
	}
}

// overlapAnswerer flags concurrent entry into Answer.
type overlapAnswerer struct {
	inFlight   atomic.Int32
	overlapped atomic.Bool
}

func (o *overlapAnswerer) Answer(_ context.Context, _ memory.Query) (memory.Answer, error) {
	if o.inFlight.Add(1) > 1 {
		o.overlapped.Store(true)
	}
	time.Sleep(20 * time.Millisecond)
	o.inFlight.Add(-1)
	return memory.Answer{Text: "x"}, nil
}

func TestSerialAnswerer_Serializes(t *testing.T) {
	inner := &overlapAnswerer{}
	serial := NewSerialAnswerer(inner)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = serial.Answer(context.Background(), memory.Query{Text: "q"})
		}()
	}
	wg.Wait()
	if inner.overlapped.Load() {
		t.Fatal("SerialAnswerer must not allow concurrent Answer calls (single pgx conn)")
	}
}

// TestChat_RejectsOversizedBody guards the body-size cap: /chat binds all
// interfaces, so an unauthenticated client must not be able to push an unbounded
// request body into the JSON decoder. The oversized request is rejected before
// the (serialized, single-conn) answerer is ever reached.
func TestChat_RejectsOversizedBody(t *testing.T) {
	stub := &stubAnswerer{out: memory.Answer{Text: "should not be reached"}}
	srv := newRAGServer(t, stub)

	// Valid JSON shape, but well over the 1 MiB cap.
	big := `{"query":"` + strings.Repeat("a", 2<<20) + `"}`
	resp := postChat(t, srv, big)
	defer resp.Body.Close()

	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Fatalf("oversized body: got status %d, want 4xx", resp.StatusCode)
	}
	if stub.calls != 0 {
		t.Fatalf("answerer must not be called for an oversized body; calls=%d", stub.calls)
	}
}
