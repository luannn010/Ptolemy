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

func TestMMR_DropsNearDuplicates(t *testing.T) {
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: "the GC sweep archives stale project rows below the decay threshold"}, Score: 1.0},
		{Chunk: Chunk{ID: "a2", Content: "the GC sweep archives stale project rows below the decay threshold"}, Score: 0.99},
		{Chunk: Chunk{ID: "b", Content: "recall isolates each subject so users never see another user memory"}, Score: 0.8},
	}
	b := MMRContextBuilder{Lambda: 0.7, K: 2, MaxRunes: 6000}
	pc := b.Build(Query{Text: "how does memory work"}, chunks)
	if len(pc.SourceIDs) != 2 {
		t.Fatalf("want 2 selected, got %v", pc.SourceIDs)
	}
	got := map[string]bool{pc.SourceIDs[0]: true, pc.SourceIDs[1]: true}
	if !got["b"] {
		t.Fatalf("MMR should include the distinct chunk b; got %v", pc.SourceIDs)
	}
	if got["a"] && got["a2"] {
		t.Fatalf("MMR should not pick both near-duplicates; got %v", pc.SourceIDs)
	}
}

func TestMMR_EmptyCandidates(t *testing.T) {
	b := MMRContextBuilder{Lambda: 0.7, K: 5, MaxRunes: 6000}
	pc := b.Build(Query{Text: "q"}, nil)
	if len(pc.SourceIDs) != 0 {
		t.Fatalf("empty candidates → no sources, got %v", pc.SourceIDs)
	}
}
