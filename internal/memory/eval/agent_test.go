package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
)

func TestScoreGiveUpCorrect_NegativeExpectsGiveUp(t *testing.T) {
	r := AgentResult{Question: Question{QuestionType: QuestionNegative}, GaveUp: true}
	if !scoreGiveUpCorrect(r) {
		t.Fatal("negative question + give_up should score correct")
	}
	r2 := AgentResult{Question: Question{QuestionType: QuestionNegative}, GaveUp: false}
	if scoreGiveUpCorrect(r2) {
		t.Fatal("negative question + answer should score incorrect")
	}
}

func TestScoreGiveUpCorrect_KnownExpectsAnswer(t *testing.T) {
	r := AgentResult{
		Question:  Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"d"}},
		GaveUp:    false,
		Citations: []string{"d#0"},
	}
	if !scoreGiveUpCorrect(r) {
		t.Fatal("known question + answer should score correct")
	}
	r2 := AgentResult{Question: Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"d"}}, GaveUp: true}
	if scoreGiveUpCorrect(r2) {
		t.Fatal("known question + give_up should score incorrect")
	}
}

func TestSummarizeAgent_Aggregates(t *testing.T) {
	results := []AgentResult{
		{Question: Question{QuestionType: QuestionNegative}, GaveUp: true},
		{Question: Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"d"}}, GaveUp: false, Citations: []string{"d#0"}},
		{Question: Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"e"}}, GaveUp: false, Citations: nil}, // answered but no citation
	}
	s := SummarizeAgent(results)
	if s.Total != 3 {
		t.Fatalf("Total = %d, want 3", s.Total)
	}
	if s.GiveUpCorrect != 3 {
		t.Fatalf("GiveUpCorrect = %d, want 3 (all matched expectation)", s.GiveUpCorrect)
	}
	if s.Answered != 2 {
		t.Fatalf("Answered = %d, want 2", s.Answered)
	}
	if s.Grounded != 1 {
		t.Fatalf("Grounded = %d, want 1 (only the one with a citation)", s.Grounded)
	}
}

// fakeAnswerer returns a canned memory.Answer keyed by query text.
type fakeAnswerer struct {
	responses map[string]memory.Answer
	err       error
}

func (f *fakeAnswerer) Answer(_ context.Context, q memory.Query) (memory.Answer, error) {
	if f.err != nil {
		return memory.Answer{}, f.err
	}
	return f.responses[q.Text], nil
}

func TestRunAgentEval_MapsAnswerFields(t *testing.T) {
	a := &fakeAnswerer{responses: map[string]memory.Answer{
		"q1": {GaveUp: true},
		"q2": {GaveUp: false, Citations: []string{"d#0"}},
	}}
	seed := Seed{K: 5, Questions: []Question{
		{ID: "i1", Text: "q1"},
		{ID: "i2", Text: "q2"},
	}}
	results, err := RunAgentEval(context.Background(), a, seed)
	if err != nil {
		t.Fatalf("RunAgentEval: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].GaveUp || results[0].Question.ID != "i1" {
		t.Fatalf("result[0] mismatch: %+v", results[0])
	}
	if results[1].GaveUp || len(results[1].Citations) != 1 || results[1].Citations[0] != "d#0" {
		t.Fatalf("result[1] mismatch: %+v", results[1])
	}
}

func TestRunAgentEval_PropagatesError(t *testing.T) {
	a := &fakeAnswerer{err: errors.New("brain down")}
	seed := Seed{K: 5, Questions: []Question{{ID: "i1", Text: "q1"}}}
	if _, err := RunAgentEval(context.Background(), a, seed); err == nil {
		t.Fatal("expected RunAgentEval to propagate the answerer error")
	}
}
