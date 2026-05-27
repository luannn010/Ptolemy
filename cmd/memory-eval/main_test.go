package main

import (
	"testing"

	"github.com/luannn010/ptolemy/internal/memory/eval"
)

func TestFilterByType_KeepsMatching(t *testing.T) {
	qs := []eval.Question{
		{ID: "q1", QuestionType: eval.QuestionParaphrase},
		{ID: "q2", QuestionType: eval.QuestionExactToken},
		{ID: "q3", QuestionType: eval.QuestionParaphrase},
		{ID: "q4", QuestionType: eval.QuestionFreshVsStale},
	}
	got := filterByType(qs, eval.QuestionParaphrase)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].ID != "q1" || got[1].ID != "q3" {
		t.Fatalf("expected q1,q3 got %+v", got)
	}
}

func TestFilterByType_EmptyResult(t *testing.T) {
	qs := []eval.Question{{ID: "q1", QuestionType: eval.QuestionParaphrase}}
	got := filterByType(qs, eval.QuestionNegative)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}
