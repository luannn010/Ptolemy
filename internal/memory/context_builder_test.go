package memory

import (
	"strings"
	"testing"
)

func TestBudgetContextBuilder_RespectsTokenBudget(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 90}
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: strings.Repeat("x", 30)}},
		{Chunk: Chunk{ID: "b", Content: strings.Repeat("y", 30)}},
		{Chunk: Chunk{ID: "c", Content: strings.Repeat("z", 30)}},
	}
	pc := cb.Build(Query{Text: "q"}, chunks)
	if len(pc.SourceIDs) != 2 {
		t.Fatalf("expected 2 chunks within budget, got %v", pc.SourceIDs)
	}
	if !strings.Contains(pc.User, "q") {
		t.Fatalf("question must appear in user prompt: %q", pc.User)
	}
}

func TestBudgetContextBuilder_AlwaysIncludesAtLeastOne(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 10}
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: strings.Repeat("x", 100)}},
	}
	pc := cb.Build(Query{Text: "q"}, chunks)
	if len(pc.SourceIDs) != 1 {
		t.Fatalf("must include at least one chunk even over budget")
	}
}

func TestBudgetContextBuilder_EmptyChunks(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 100}
	pc := cb.Build(Query{Text: "q"}, nil)
	if len(pc.SourceIDs) != 0 {
		t.Fatalf("expected no source IDs for empty input")
	}
	if !strings.Contains(pc.User, "q") {
		t.Fatalf("user prompt should still contain the question")
	}
}

func TestBudgetContextBuilder_IncludesSystemPrompt(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 100}
	pc := cb.Build(Query{Text: "q"}, []RetrievedChunk{{Chunk: Chunk{ID: "a", Content: "x"}}})
	if pc.System == "" {
		t.Fatalf("system prompt must not be empty")
	}
	if !strings.Contains(pc.System, "source") {
		t.Fatalf("system prompt should instruct on source citation, got %q", pc.System)
	}
}

func TestBudgetContextBuilder_ZeroBudgetIncludesAll(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 0}
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: "x"}},
		{Chunk: Chunk{ID: "b", Content: "y"}},
	}
	pc := cb.Build(Query{Text: "q"}, chunks)
	if len(pc.SourceIDs) != 2 {
		t.Fatalf("MaxRunes=0 should disable the budget, got %d ids", len(pc.SourceIDs))
	}
}
