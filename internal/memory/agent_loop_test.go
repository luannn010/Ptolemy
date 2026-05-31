package memory

import (
	"context"
	"testing"
)

// stubPlanner returns a scripted sequence of actions; once exhausted it keeps
// returning a non-terminal retrieve (to exercise budget exhaustion).
type stubPlanner struct {
	actions []AgentAction
	i       int
}

func (s *stubPlanner) NextAction(_ context.Context, _ AgentState) (AgentAction, error) {
	if s.i >= len(s.actions) {
		return AgentAction{Type: ActionRetrieve, Query: "more"}, nil
	}
	a := s.actions[s.i]
	s.i++
	return a, nil
}

// stubRetriever returns a fixed batch regardless of query/depth.
type stubRetriever struct{ chunks []RetrievedChunk }

func (s stubRetriever) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	return s.chunks, nil
}

func chunk(id, content string) RetrievedChunk {
	return RetrievedChunk{Chunk: Chunk{ID: id, Content: content}}
}

func TestAgentLoop_BudgetExhaustionForcesGiveUp(t *testing.T) {
	loop := &AgentLoop{
		Planner:   &stubPlanner{}, // always retrieve, never terminal
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "x")}},
		Cfg:       AgentConfig{MaxSteps: 3},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.GaveUp {
		t.Fatalf("expected give_up at budget exhaustion, got %+v", ans)
	}
}

func TestAgentLoop_GiveUpActionShortCircuits(t *testing.T) {
	loop := &AgentLoop{
		Planner:   &stubPlanner{actions: []AgentAction{{Type: ActionGiveUp, Reason: "not in KB"}}},
		Retriever: stubRetriever{},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.GaveUp {
		t.Fatalf("expected give_up, got %+v", ans)
	}
}

// countingRetriever returns a distinct single chunk on each call, so the test
// can assert that successive retrieves accumulate rather than replace.
type countingRetriever struct{ n int }

func (c *countingRetriever) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	c.n++
	return []RetrievedChunk{chunk("c"+string(rune('0'+c.n))+"#0", "x")}, nil
}

func TestAgentLoop_TwoRetrieves_ChunksAccumulate(t *testing.T) {
	var captured AgentState
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionRetrieve, Query: "b"},
			{Type: ActionGiveUp, Reason: "stop"},
		}},
		Retriever: &countingRetriever{},
		Cfg:       AgentConfig{MaxSteps: 5},
		onState:   func(s AgentState) { captured = s },
	}
	if _, err := loop.Run(context.Background(), Query{Text: "q"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured.AccumulatedChunks) != 2 {
		t.Fatalf("expected 2 accumulated chunks after 2 retrieves, got %d", len(captured.AccumulatedChunks))
	}
}

// stubGenerator returns a fixed answer + citations and counts calls.
type stubGenerator struct {
	text  string
	cites []string
	calls int
}

func (g *stubGenerator) Generate(_ context.Context, _ Query, _ PromptContext) (Answer, error) {
	g.calls++
	return Answer{Text: g.text, Citations: g.cites}, nil
}

func TestAgentLoop_AnswerWithoutChunks_GivesUp(t *testing.T) {
	gen := &stubGenerator{text: "anything"}
	loop := &AgentLoop{
		Planner:   &stubPlanner{actions: []AgentAction{{Type: ActionAnswer}}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.GaveUp {
		t.Fatalf("answer with no chunks must give_up, got %+v", ans)
	}
	if gen.calls != 0 {
		t.Fatalf("generator must not be called with no chunks, calls=%d", gen.calls)
	}
}

func TestGroundingCheck_CitationNotInChunks_FailsGrounding(t *testing.T) {
	if isGrounded("text [source:ghost] more", []string{"real"}) {
		t.Fatal("citation not in chunks must fail grounding")
	}
	if !isGrounded("text [source:real] more", []string{"real"}) {
		t.Fatal("citation present in chunks must pass grounding")
	}
	if isGrounded("no citations at all", []string{"real"}) {
		t.Fatal("an answer with zero citations is ungrounded")
	}
	// Mixed valid + invalid citation in one answer → ungrounded (any miss fails).
	if isGrounded("a [source:real] b [source:ghost]", []string{"real"}) {
		t.Fatal("a single invalid citation among valid ones must fail grounding")
	}
	// Duplicate valid citation → grounded (dedup is not required for grounding).
	if !isGrounded("[source:real] [source:real]", []string{"real"}) {
		t.Fatal("duplicate valid citations must still pass grounding")
	}
}

func TestAgentLoop_RetrieveThenAnswer_HappyPath(t *testing.T) {
	gen := &stubGenerator{text: "the answer [source:a#0]", cites: []string{"a#0"}}
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "grounded fact")}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.GaveUp {
		t.Fatalf("happy path must not give_up: %+v", ans)
	}
	if gen.calls != 1 {
		t.Fatalf("generator should be called once, calls=%d", gen.calls)
	}
	if len(ans.Citations) != 1 || ans.Citations[0] != "a#0" {
		t.Fatalf("expected citation a#0, got %v", ans.Citations)
	}
}

func TestAgentLoop_UngroundedAnswer_BecomesGiveUp(t *testing.T) {
	// generator drops the ghost citation (not in SourceIDs) → returns no citations,
	// and its text cites only ghost → grounding fails.
	gen := &stubGenerator{text: "fabricated [source:ghost]", cites: nil}
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "real fact")}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.GaveUp {
		t.Fatalf("ungrounded answer must become give_up, got %+v", ans)
	}
}

// Also cover the unknown-action defensive branch (reviewer-requested for Task 5).
func TestAgentLoop_UnknownAction_Errors(t *testing.T) {
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{{Type: "frobnicate"}}},
		Cfg:     AgentConfig{MaxSteps: 5},
	}
	if _, err := loop.Run(context.Background(), Query{Text: "q"}); err == nil {
		t.Fatal("expected error for unknown action type")
	}
}
