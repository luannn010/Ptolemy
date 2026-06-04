package memorytools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
)

// --- fakes -------------------------------------------------------------------

type fakeRecaller struct {
	gotQuery    memory.Query
	answer      memory.Answer
	recall      memory.RecallResult
	err         error
	answerCalls int
	recallCalls int
}

func (f *fakeRecaller) Answer(_ context.Context, q memory.Query) (memory.Answer, error) {
	f.gotQuery = q
	f.answerCalls++
	return f.answer, f.err
}

func (f *fakeRecaller) Recall(_ context.Context, q memory.Query) (memory.RecallResult, error) {
	f.gotQuery = q
	f.recallCalls++
	return f.recall, f.err
}

type fakeCapturer struct {
	gotEx memory.Exchange
	err   error
}

func (f *fakeCapturer) Capture(_ context.Context, ex memory.Exchange) error {
	f.gotEx = ex
	return f.err
}

type fakeConsolidator struct {
	gotSubject string
	gotProject string
	err        error
}

func (f *fakeConsolidator) ConsolidateSubjectProject(_ context.Context, subject, project string) error {
	f.gotSubject, f.gotProject = subject, project
	return f.err
}

func textOf(t *testing.T, res map[string]any) string {
	t.Helper()
	content, ok := res["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content []map[string]any, got %#v", res["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	return content[0]["text"].(string)
}

// --- Tools() registration ----------------------------------------------------

func TestToolsRegistered(t *testing.T) {
	tools := Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 memory tools, got %d", len(tools))
	}
	want := map[string]bool{
		"ptolemy_memory_recall":      false,
		"ptolemy_memory_capture":     false,
		"ptolemy_memory_consolidate": false,
	}
	for _, tl := range tools {
		if _, ok := want[tl.Name]; !ok {
			t.Fatalf("unexpected tool %q", tl.Name)
		}
		want[tl.Name] = true
		if _, ok := tl.InputSchema["properties"].(map[string]any); !ok {
			t.Fatalf("tool %q missing properties schema", tl.Name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing tool %q", name)
		}
	}
}

func TestRecallToolRequiresQuery(t *testing.T) {
	for _, tl := range Tools() {
		if tl.Name != "ptolemy_memory_recall" {
			continue
		}
		req, ok := tl.InputSchema["required"].([]string)
		if !ok {
			t.Fatalf("expected required []string, got %#v", tl.InputSchema["required"])
		}
		if len(req) != 1 || req[0] != "query" {
			t.Fatalf("expected recall to require only query, got %v", req)
		}
	}
}

// --- recall ------------------------------------------------------------------

func TestHandleRecall(t *testing.T) {
	rec := &fakeRecaller{recall: memory.RecallResult{Context: "BRAIN is on port 1090", SourceIDs: []string{"turn:abc"}}}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	res, handled, err := h("ptolemy_memory_recall", map[string]any{"query": "which port"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	// Default is the fast retrieval-only path (no LLM).
	if rec.recallCalls != 1 || rec.answerCalls != 0 {
		t.Fatalf("expected Recall by default, got recall=%d answer=%d", rec.recallCalls, rec.answerCalls)
	}
	if rec.gotQuery.Text != "which port" {
		t.Fatalf("query text not passed: %q", rec.gotQuery.Text)
	}
	// subject/project default from Deps
	if rec.gotQuery.SubjectID == nil || *rec.gotQuery.SubjectID != "luan" {
		t.Fatalf("expected default subject luan, got %v", rec.gotQuery.SubjectID)
	}
	if rec.gotQuery.ProjectID == nil || *rec.gotQuery.ProjectID != "ptolemy" {
		t.Fatalf("expected default project ptolemy, got %v", rec.gotQuery.ProjectID)
	}

	var out struct {
		Text      string   `json:"text"`
		Citations []string `json:"citations"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out.Text != "BRAIN is on port 1090" || len(out.Citations) != 1 {
		t.Fatalf("unexpected result payload: %#v", out)
	}
}

func TestHandleRecallGenerateUsesLLM(t *testing.T) {
	rec := &fakeRecaller{answer: memory.Answer{Text: "prose", Citations: []string{"c1"}}}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	res, _, err := h("ptolemy_memory_recall", map[string]any{"query": "q", "generate": true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.answerCalls != 1 || rec.recallCalls != 0 {
		t.Fatalf("expected Answer when generate=true, got recall=%d answer=%d", rec.recallCalls, rec.answerCalls)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out.Text != "prose" {
		t.Fatalf("unexpected payload: %#v", out)
	}
}

func TestHandleRecallArgsOverrideDefaults(t *testing.T) {
	rec := &fakeRecaller{}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	_, _, err := h("ptolemy_memory_recall", map[string]any{
		"query":      "q",
		"subject_id": "other",
		"project_id": "proj2",
		"k":          float64(3), // JSON numbers decode to float64
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *rec.gotQuery.SubjectID != "other" || *rec.gotQuery.ProjectID != "proj2" {
		t.Fatalf("args did not override defaults: %v %v", rec.gotQuery.SubjectID, rec.gotQuery.ProjectID)
	}
	if rec.gotQuery.K != 3 {
		t.Fatalf("expected k=3, got %d", rec.gotQuery.K)
	}
}

func TestHandleRecallMissingQuery(t *testing.T) {
	h := NewHandler(Deps{Recall: &fakeRecaller{}})
	_, handled, err := h("ptolemy_memory_recall", map[string]any{}, nil)
	if !handled {
		t.Fatal("expected handled=true even on bad args")
	}
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

// --- capture -----------------------------------------------------------------

func TestHandleCapture(t *testing.T) {
	cap := &fakeCapturer{}
	h := NewHandler(Deps{Capture: cap, Subject: "luan", Project: "ptolemy"})

	res, handled, err := h("ptolemy_memory_capture", map[string]any{
		"user_text":      "BRAIN runs on 1090",
		"assistant_text": "noted",
		"session_id":     "sess-1",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if cap.gotEx.UserText != "BRAIN runs on 1090" || cap.gotEx.AssistantText != "noted" {
		t.Fatalf("exchange text not passed: %#v", cap.gotEx)
	}
	if cap.gotEx.SubjectID != "luan" || cap.gotEx.ProjectID != "ptolemy" || cap.gotEx.SessionID != "sess-1" {
		t.Fatalf("scoping not applied: %#v", cap.gotEx)
	}
	var out struct {
		Captured bool `json:"captured"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if !out.Captured {
		t.Fatal("expected captured=true")
	}
}

func TestHandleCaptureRequiresText(t *testing.T) {
	h := NewHandler(Deps{Capture: &fakeCapturer{}})
	_, handled, err := h("ptolemy_memory_capture", map[string]any{}, nil)
	if !handled {
		t.Fatal("expected handled=true")
	}
	if err == nil {
		t.Fatal("expected error when both user_text and assistant_text are empty")
	}
}

func TestHandleCapturePropagatesError(t *testing.T) {
	h := NewHandler(Deps{Capture: &fakeCapturer{err: errors.New("boom")}})
	_, _, err := h("ptolemy_memory_capture", map[string]any{"user_text": "x"}, nil)
	if err == nil {
		t.Fatal("expected capture error to propagate")
	}
}

// --- consolidate -------------------------------------------------------------

func TestHandleConsolidate(t *testing.T) {
	con := &fakeConsolidator{}
	h := NewHandler(Deps{Consolidate: con, Subject: "luan", Project: "ptolemy"})

	res, handled, err := h("ptolemy_memory_consolidate", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if con.gotSubject != "luan" || con.gotProject != "ptolemy" {
		t.Fatalf("defaults not applied: %q %q", con.gotSubject, con.gotProject)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
}

// --- recall trace ------------------------------------------------------------

func TestHandleRecallTraceReturnsSteps(t *testing.T) {
	rec := &fakeRecaller{answer: memory.Answer{
		Text:      "prose [source:a#0]",
		Citations: []string{"a#0"},
		Trace: &memory.RecallTrace{
			Mode: "agentic",
			Steps: []memory.TraceStep{
				{Index: 0, Action: "retrieve", Query: "terms", Retrieved: []memory.TraceChunk{{ID: "a#0", Score: 0.5, Snippet: "fact"}}},
				{Index: 1, Action: "answer", GroundingOK: true},
			},
		},
	}}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	res, _, err := h("ptolemy_memory_recall", map[string]any{"query": "q", "trace": true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// trace=true must route through Answer (the generate path) and set Query.Trace.
	if rec.answerCalls != 1 || rec.recallCalls != 0 {
		t.Fatalf("trace=true must use Answer, got recall=%d answer=%d", rec.recallCalls, rec.answerCalls)
	}
	if !rec.gotQuery.Trace {
		t.Fatal("expected Query.Trace=true passed through")
	}
	var out struct {
		Text  string `json:"text"`
		Mode  string `json:"mode"`
		Steps []struct {
			Action    string `json:"action"`
			Query     string `json:"query"`
			Retrieved []struct {
				ID      string  `json:"id"`
				Score   float64 `json:"score"`
				Snippet string  `json:"snippet"`
			} `json:"retrieved"`
			GroundingOK bool `json:"grounding_ok"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out.Mode != "agentic" || len(out.Steps) != 2 {
		t.Fatalf("unexpected steps payload: %#v", out)
	}
	if out.Steps[0].Action != "retrieve" || len(out.Steps[0].Retrieved) != 1 || out.Steps[0].Retrieved[0].ID != "a#0" {
		t.Fatalf("step0 wrong: %#v", out.Steps[0])
	}
	if !out.Steps[1].GroundingOK {
		t.Fatalf("step1 grounding flag missing: %#v", out.Steps[1])
	}
}

func TestHandleRecallNoTraceOmitsSteps(t *testing.T) {
	rec := &fakeRecaller{recall: memory.RecallResult{Context: "ctx", SourceIDs: []string{"a#0"}}}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	res, _, err := h("ptolemy_memory_recall", map[string]any{"query": "q"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Regression: default path unchanged — retrieval-only, no steps key.
	if rec.recallCalls != 1 || rec.answerCalls != 0 {
		t.Fatalf("default must stay retrieval-only, got recall=%d answer=%d", rec.recallCalls, rec.answerCalls)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(textOf(t, res)), &raw); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, ok := raw["steps"]; ok {
		t.Fatalf("steps must be absent without trace=true, got %#v", raw)
	}
}

// --- routing -----------------------------------------------------------------

func TestHandleUnknownTool(t *testing.T) {
	h := NewHandler(Deps{})
	res, handled, err := h("not_a_memory_tool", map[string]any{}, nil)
	if handled || err != nil || res != nil {
		t.Fatalf("expected passthrough (nil,false,nil), got %#v %v %v", res, handled, err)
	}
}
