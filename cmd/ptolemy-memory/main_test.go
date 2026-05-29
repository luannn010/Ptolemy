package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
)

type fakeRecaller struct {
	got    memory.Query
	answer memory.Answer
	err    error
}

func (f *fakeRecaller) Answer(_ context.Context, q memory.Query) (memory.Answer, error) {
	f.got = q
	return f.answer, f.err
}

type fakeCapturer struct {
	got memory.Exchange
	err error
}

func (f *fakeCapturer) Capture(_ context.Context, ex memory.Exchange) error {
	f.got = ex
	return f.err
}

func TestRunRecallPrintsSummary(t *testing.T) {
	r := &fakeRecaller{answer: memory.Answer{Text: "BRAIN is on port 1090", Citations: []string{"synth:1"}}}
	var out bytes.Buffer
	err := runRecall(context.Background(), r, recallOpts{Query: "which port", Subject: "luan", Project: "ptolemy", K: 5}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.got.Text != "which port" || *r.got.SubjectID != "luan" || *r.got.ProjectID != "ptolemy" || r.got.K != 5 {
		t.Fatalf("query not built correctly: %#v", r.got)
	}
	if !strings.Contains(out.String(), "BRAIN is on port 1090") {
		t.Fatalf("summary not printed: %q", out.String())
	}
}

func TestRunRecallEmptyQueryRecallsProjectContext(t *testing.T) {
	// SessionStart use: no --query means "recall this project's context".
	r := &fakeRecaller{answer: memory.Answer{Text: "project summary"}}
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
