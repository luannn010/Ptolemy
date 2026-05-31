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
