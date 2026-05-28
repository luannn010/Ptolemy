package memory

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeStore struct {
	upserted       []Chunk
	supersedeCalls []supersedeCall
	reinforced     [][]string // records each Reinforce call's ids
	statsCalls     int
}

type supersedeCall struct {
	OldDocID  string
	NewChunks []Chunk
}

func (f *fakeStore) Upsert(_ context.Context, chunks []Chunk) error {
	f.upserted = append(f.upserted, chunks...)
	return nil
}
func (f *fakeStore) Get(_ context.Context, _ []string) ([]Chunk, error) { return nil, nil }
func (f *fakeStore) MarkSuperseded(_ context.Context, _, _ string) error {
	return nil
}
func (f *fakeStore) SupersedeOnUpsert(_ context.Context, chunks []Chunk, oldDocID string) error {
	f.upserted = append(f.upserted, chunks...)
	f.supersedeCalls = append(f.supersedeCalls, supersedeCall{OldDocID: oldDocID, NewChunks: chunks})
	return nil
}

func (f *fakeStore) Reinforce(_ context.Context, ids []string) error {
	f.reinforced = append(f.reinforced, ids)
	return nil
}

func (f *fakeStore) Stats(_ context.Context) ([]ScopeStatusCount, error) {
	f.statsCalls++
	return nil, nil
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

type erroringRetriever struct{}

func (erroringRetriever) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	return nil, fmt.Errorf("backend down")
}

func TestOrchestrator_AnswerPropagatesRetrieverError(t *testing.T) {
	o := &Orchestrator{
		Retriever:      erroringRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	if _, err := o.Answer(context.Background(), Query{Text: "q", K: 1}); err == nil {
		t.Fatalf("expected retriever error to surface")
	}
}

func TestOrchestrator_AnswerDefaultsDepthAndFinalK(t *testing.T) {
	// Depth=0 and Query.K=0 + FinalK=0 should all use the built-in defaults
	// instead of returning early or panicking.
	o := &Orchestrator{
		Retriever:      fakeRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		// Depth and FinalK intentionally zero.
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q"}) // Query.K=0
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if ans.Text == "" {
		t.Fatalf("expected non-empty answer when depth/finalK default")
	}
}

type erroringEmbedder struct{}

func (erroringEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embed down")
}

func TestOrchestrator_IngestPropagatesEmbedError(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: erroringEmbedder{},
		Store:    store,
	}
	if err := o.Ingest(context.Background(), RawDocument{ID: "d", Text: "hello"}); err == nil {
		t.Fatalf("expected embed error to surface")
	}
	if len(store.upserted) != 0 {
		t.Fatalf("nothing should be stored when embedder errors")
	}
}

func TestOrchestrator_IngestIgnoresUnparseablePublishedAt(t *testing.T) {
	// A malformed published_at metadata value must fall back to time.Now(),
	// not abort the ingest or surface as an error.
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1}}},
		Store:    store,
	}
	before := time.Now().Add(-time.Second)
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d", Text: "hi",
		Metadata: map[string]any{"published_at": "not-a-timestamp"},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 chunk stored, got %d", len(store.upserted))
	}
	if store.upserted[0].PublishedAt.Before(before) {
		t.Fatalf("expected fallback to time.Now(), got %v", store.upserted[0].PublishedAt)
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

func TestOrchestrator_Ingest_SupersedesOldDoc(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID:   "v2",
		Text: "new content",
		Metadata: map[string]any{
			"supersedes": "v1",
		},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.supersedeCalls) != 1 {
		t.Fatalf("expected 1 SupersedeOnUpsert call, got %d", len(store.supersedeCalls))
	}
	if store.supersedeCalls[0].OldDocID != "v1" {
		t.Fatalf("expected OldDocID=v1, got %q", store.supersedeCalls[0].OldDocID)
	}
	if len(store.supersedeCalls[0].NewChunks) != 1 || store.supersedeCalls[0].NewChunks[0].ID != "v2#0" {
		t.Fatalf("expected NewChunks=[v2#0], got %+v", store.supersedeCalls[0].NewChunks)
	}
}

func TestOrchestrator_Ingest_NoSupersedesUsesPlainUpsert(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID:   "d",
		Text: "first ingest",
		// No "supersedes" key.
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.supersedeCalls) != 0 {
		t.Fatalf("expected 0 SupersedeOnUpsert calls, got %d", len(store.supersedeCalls))
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
}

func TestOrchestrator_Ingest_NonStringSupersedesIsIgnored(t *testing.T) {
	// A malformed metadata value (e.g. a number) MUST NOT crash and MUST fall
	// back to plain Upsert. This protects the orchestrator from caller bugs.
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID:   "d",
		Text: "anything",
		Metadata: map[string]any{
			"supersedes": 42, // not a string
		},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.supersedeCalls) != 0 {
		t.Fatalf("non-string supersedes must be ignored; got %d calls", len(store.supersedeCalls))
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 plain upsert, got %d", len(store.upserted))
	}
}

// capturingRetriever records the last Query it received so tests can assert
// the orchestrator's resolution of optional fields (AsOf, K, etc).
type capturingRetriever struct {
	lastQuery Query
}

func (c *capturingRetriever) Retrieve(_ context.Context, q Query, _ int) ([]RetrievedChunk, error) {
	c.lastQuery = q
	return []RetrievedChunk{
		{Chunk: Chunk{ID: "c1", Content: "anything"}, Score: 0.5},
	}, nil
}

func TestOrchestrator_Answer_AsOfNilDefaultsToNow(t *testing.T) {
	r := &capturingRetriever{}
	o := &Orchestrator{
		Retriever:      r,
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	before := time.Now().UTC().Add(-time.Second)
	if _, err := o.Answer(context.Background(), Query{Text: "q", K: 1}); err != nil {
		t.Fatal(err)
	}
	if r.lastQuery.AsOf == nil {
		t.Fatalf("retriever received Query with AsOf=nil; expected populated default")
	}
	if r.lastQuery.AsOf.Before(before) {
		t.Fatalf("AsOf default %v should be ~now, before=%v", *r.lastQuery.AsOf, before)
	}
}

func TestOrchestrator_Answer_AsOfRespected(t *testing.T) {
	r := &capturingRetriever{}
	o := &Orchestrator{
		Retriever:      r,
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	pin := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := o.Answer(context.Background(), Query{Text: "q", K: 1, AsOf: &pin}); err != nil {
		t.Fatal(err)
	}
	if r.lastQuery.AsOf == nil || !r.lastQuery.AsOf.Equal(pin) {
		t.Fatalf("expected AsOf=%v, got %v", pin, r.lastQuery.AsOf)
	}
}

type fakeRetriever2 struct{ out []RetrievedChunk }

func (f *fakeRetriever2) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	return f.out, nil
}

func TestOrchestrator_Ingest_ScopeDefaultsGlobal(t *testing.T) {
	fs := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}},
		Store:    fs,
	}
	if err := o.Ingest(context.Background(), RawDocument{ID: "d", Text: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if len(fs.upserted) == 0 || fs.upserted[0].Scope != "global" {
		t.Fatalf("expected scope 'global', got %+v", fs.upserted)
	}
}

func TestOrchestrator_Ingest_ScopeFromMetadata(t *testing.T) {
	fs := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}},
		Store:    fs,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d", Text: "alpha", Metadata: map[string]any{"scope": "project"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.upserted) == 0 || fs.upserted[0].Scope != "project" {
		t.Fatalf("expected scope 'project', got %+v", fs.upserted)
	}
}

func TestOrchestrator_Answer_Reinforces(t *testing.T) {
	fs := &fakeStore{}
	o := &Orchestrator{
		Embedder:       fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}},
		Store:          fs,
		Retriever:      &fakeRetriever2{out: []RetrievedChunk{{Chunk: Chunk{ID: "c1#0"}}, {Chunk: Chunk{ID: "c2#0"}}}},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Generator:      fakeGenerator{},
		Depth:          20,
		FinalK:         5,
	}
	if _, err := o.Answer(context.Background(), Query{Text: "alpha", K: 5}); err != nil {
		t.Fatal(err)
	}
	if len(fs.reinforced) != 1 {
		t.Fatalf("expected exactly one Reinforce call, got %d", len(fs.reinforced))
	}
	if len(fs.reinforced[0]) != 2 {
		t.Fatalf("expected 2 ids reinforced, got %v", fs.reinforced[0])
	}
}
