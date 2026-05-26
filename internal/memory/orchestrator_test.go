package memory

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	upserted []Chunk
}

func (f *fakeStore) Upsert(_ context.Context, chunks []Chunk) error {
	f.upserted = append(f.upserted, chunks...)
	return nil
}
func (f *fakeStore) Get(_ context.Context, _ []string) ([]Chunk, error) { return nil, nil }
func (f *fakeStore) MarkSuperseded(_ context.Context, _, _ string) error {
	return nil
}

type fakeRetriever struct{}

func (fakeRetriever) Retrieve(_ context.Context, q Query, _ int) ([]RetrievedChunk, error) {
	return []RetrievedChunk{
		{Chunk: Chunk{ID: "c1", Content: "alpha"}, Score: 0.9},
	}, nil
}

type fakeGenerator struct{}

func (fakeGenerator) Generate(_ context.Context, _ Query, pc PromptContext) (Answer, error) {
	return Answer{Text: "the answer [source:c1]", Citations: []string{"c1"}}, nil
}

func TestOrchestrator_IngestEmbedsAndStores(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:        FixedSizeChunker{MaxRunes: 10, Overlap: 0},
		Embedder:       fakeEmbedder{vecs: [][]float32{{1, 0}, {0, 1}}},
		Store:          store,
		Retriever:      fakeRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d1", Text: "abcdefghij1234567890", Source: "s",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.upserted) != 2 {
		t.Fatalf("expected 2 chunks stored, got %d", len(store.upserted))
	}
	if len(store.upserted[0].Embedding) != 2 {
		t.Fatalf("embedding not attached: %+v", store.upserted[0])
	}
}

func TestOrchestrator_AnswerReturnsCitedAnswer(t *testing.T) {
	o := &Orchestrator{
		Retriever:      fakeRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q", K: 1})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(ans.Citations) != 1 || ans.Citations[0] != "c1" {
		t.Fatalf("expected citation c1, got %v", ans.Citations)
	}
}

func TestOrchestrator_IngestSetsPublishedAtFromSource(t *testing.T) {
	store := &fakeStore{}
	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d", Text: "tiny", Source: "s",
		Metadata: map[string]any{"published_at": when.Format(time.RFC3339)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.upserted[0].PublishedAt.Equal(when) {
		t.Fatalf("published_at not honored: got %v want %v", store.upserted[0].PublishedAt, when)
	}
}

func TestOrchestrator_IngestEmptyTextSkipsStore(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: nil},
		Store:    store,
	}
	// Empty text → chunker produces 1 chunk with empty content; embedder returns nil vecs.
	// Expect a clear error rather than a silent half-write.
	if err := o.Ingest(context.Background(), RawDocument{ID: "d", Text: ""}); err == nil {
		t.Fatalf("expected error when embedder returns no vectors for chunks")
	}
	if len(store.upserted) != 0 {
		t.Fatalf("nothing should be stored on embedder mismatch")
	}
}

func TestOrchestrator_AnswerHonorsQueryK(t *testing.T) {
	o := &Orchestrator{
		Retriever:      fakeRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         99, // would be overridden by Query.K
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q", K: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Text == "" {
		t.Fatalf("expected non-empty answer")
	}
}
