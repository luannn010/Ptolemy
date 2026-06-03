package memory

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
)

type fakeRecaller struct {
	got         memory.Query
	answer      memory.Answer
	recall      memory.RecallResult
	err         error
	answerCalls int
	recallCalls int
}

func (f *fakeRecaller) Answer(_ context.Context, q memory.Query) (memory.Answer, error) {
	f.got = q
	f.answerCalls++
	return f.answer, f.err
}

func (f *fakeRecaller) Recall(_ context.Context, q memory.Query) (memory.RecallResult, error) {
	f.got = q
	f.recallCalls++
	return f.recall, f.err
}

type fakeCapturer struct {
	got memory.Exchange
	err error
}

func (f *fakeCapturer) Capture(_ context.Context, ex memory.Exchange) error {
	f.got = ex
	return f.err
}

func TestRunRecallDefaultsToFastRetrieval(t *testing.T) {
	r := &fakeRecaller{recall: memory.RecallResult{Context: "BRAIN is on port 1090", SourceIDs: []string{"synth:1"}}}
	var out bytes.Buffer
	err := runRecall(context.Background(), r, recallOpts{Query: "which port", Subject: "luan", Project: "ptolemy", K: 5}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.recallCalls != 1 || r.answerCalls != 0 {
		t.Fatalf("expected Recall (fast) by default, got recall=%d answer=%d", r.recallCalls, r.answerCalls)
	}
	if r.got.Text != "which port" || *r.got.SubjectID != "luan" || *r.got.ProjectID != "ptolemy" || r.got.K != 5 {
		t.Fatalf("query not built correctly: %#v", r.got)
	}
	if !strings.Contains(out.String(), "BRAIN is on port 1090") {
		t.Fatalf("context not printed: %q", out.String())
	}
}

func TestRunRecallGenerateUsesLLM(t *testing.T) {
	r := &fakeRecaller{answer: memory.Answer{Text: "prose answer", Citations: []string{"c1"}}}
	var out bytes.Buffer
	err := runRecall(context.Background(), r, recallOpts{Query: "q", Generate: true}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.answerCalls != 1 || r.recallCalls != 0 {
		t.Fatalf("expected Answer (LLM) when Generate=true, got recall=%d answer=%d", r.recallCalls, r.answerCalls)
	}
	if !strings.Contains(out.String(), "prose answer") {
		t.Fatalf("answer not printed: %q", out.String())
	}
}

func TestRunRecallEmptyQueryRecallsProjectContext(t *testing.T) {
	// SessionStart use: no --query means "recall this project's context".
	r := &fakeRecaller{recall: memory.RecallResult{Context: "project summary"}}
	var out bytes.Buffer
	err := runRecall(context.Background(), r, recallOpts{Query: "", Subject: "luan", Project: "ptolemy"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(r.got.Text) == "" {
		t.Fatal("expected a non-empty fallback query when --query is omitted")
	}
}

func TestRunCapture(t *testing.T) {
	c := &fakeCapturer{}
	err := runCapture(context.Background(), c, memory.Exchange{
		UserText: "BRAIN on 1090", AssistantText: "noted",
		SubjectID: "luan", ProjectID: "ptolemy", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.got.UserText != "BRAIN on 1090" || c.got.SubjectID != "luan" || c.got.SessionID != "s1" {
		t.Fatalf("exchange not passed: %#v", c.got)
	}
}

func TestRunCaptureRequiresText(t *testing.T) {
	err := runCapture(context.Background(), &fakeCapturer{}, memory.Exchange{SubjectID: "luan"})
	if err == nil {
		t.Fatal("expected error when both texts empty")
	}
}

func TestRunCapturePropagatesError(t *testing.T) {
	err := runCapture(context.Background(), &fakeCapturer{err: errors.New("boom")}, memory.Exchange{UserText: "x"})
	if err == nil {
		t.Fatal("expected capture error to propagate")
	}
}

// TestRunRecall_BadFlagReturnsError exercises the flag-parse guard in RunRecall
// (the exported function). Passing an unknown flag triggers fs.Parse to return
// an error before any DB is opened.
func TestRunRecall_BadFlagReturnsError(t *testing.T) {
	var out, errBuf strings.Builder
	err := RunRecall(context.Background(), []string{"--no-such-flag"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

// TestRunCapture_BadFlagReturnsError exercises the flag-parse guard in RunCapture.
func TestRunCapture_BadFlagReturnsError(t *testing.T) {
	var out, errBuf strings.Builder
	err := RunCapture(context.Background(), []string{"--no-such-flag"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
