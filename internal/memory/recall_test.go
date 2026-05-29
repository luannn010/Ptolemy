package memory

import (
	"context"
	"strings"
	"testing"
)

// bombGenerator fails the test if Generate is ever called — Recall must not
// invoke the LLM.
type bombGenerator struct{ t *testing.T }

func (g bombGenerator) Generate(_ context.Context, _ Query, _ PromptContext) (Answer, error) {
	g.t.Fatalf("Generate must not be called during Recall")
	return Answer{}, nil
}

func TestOrchestrator_RecallSkipsGenerator(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Retriever:      fakeRetriever{}, // returns c1/"alpha"
		Fusion:         PassthroughFusion{},
		ContextBuilder: MMRContextBuilder{Lambda: 0.7, K: 5, MaxRunes: 1000},
		Generator:      bombGenerator{t: t},
		Store:          store,
		Depth:          5,
		FinalK:         5,
	}
	res, err := o.Recall(context.Background(), Query{Text: "q", K: 5})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !strings.Contains(res.Context, "alpha") {
		t.Fatalf("expected recalled content to contain 'alpha', got %q", res.Context)
	}
	if len(res.SourceIDs) != 1 || res.SourceIDs[0] != "c1" {
		t.Fatalf("expected source id c1, got %v", res.SourceIDs)
	}
	// Recall still reinforces retrieved rows (GC access signal), like Answer.
	if len(store.reinforced) == 0 {
		t.Fatalf("expected Recall to reinforce retrieved rows")
	}
}

func TestOrchestrator_RecallPrefersSynthesisThenAtoms(t *testing.T) {
	ret := &fakeRetriever2{out: []RetrievedChunk{
		{Chunk: Chunk{ID: "a1", Content: "alpha", Metadata: map[string]any{"kind": "atom"}}, Score: 0.9},
		{Chunk: Chunk{ID: "syn1", Content: "the summary", Metadata: map[string]any{"kind": "synthesis"}}, Score: 0.4},
	}}
	o := &Orchestrator{
		Retriever:      ret,
		Fusion:         PassthroughFusion{},
		ContextBuilder: MMRContextBuilder{Lambda: 0.7, K: 5, MaxRunes: 1000},
		Generator:      bombGenerator{t: t},
		Store:          &fakeStore{},
		Depth:          5,
		FinalK:         5,
	}
	res, err := o.Recall(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(res.SourceIDs) == 0 || res.SourceIDs[0] != "syn1" {
		t.Fatalf("expected synthesis id first, got %v", res.SourceIDs)
	}
}
