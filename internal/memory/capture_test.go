package memory

import (
	"context"
	"sync"
	"testing"
)

type captureFakeEmbedder struct{}

func (captureFakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0, 0}
	}
	return out, nil
}

type shortVecEmbedder struct{}

func (shortVecEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return [][]float32{}, nil
}

// captureFakeStore records Upsert calls; implements just enough of Store.
type captureFakeStore struct {
	mu       sync.Mutex
	upserted []Chunk
	reinforced []string
	superseded []string
	lookupFound bool
	lookupChunk Chunk
}

func (s *captureFakeStore) Upsert(ctx context.Context, c []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserted = append(s.upserted, c...)
	return nil
}
func (s *captureFakeStore) Get(context.Context, []string) ([]Chunk, error)           { return nil, nil }
func (s *captureFakeStore) SupersedeOnUpsert(context.Context, []Chunk, string) error { return nil }
func (s *captureFakeStore) History(context.Context, string) ([]Chunk, error)         { return nil, nil }
func (s *captureFakeStore) LookupFact(context.Context, string, string) (Chunk, bool, error) {
	return s.lookupChunk, s.lookupFound, nil
}
func (s *captureFakeStore) Supersede(ctx context.Context, c []Chunk, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.superseded = append(s.superseded, id)
	return nil
}
func (s *captureFakeStore) Reinforce(ctx context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reinforced = append(s.reinforced, ids...)
	return nil
}
func (s *captureFakeStore) Stats(context.Context) ([]ScopeStatusCount, error) { return nil, nil }

func newTestHook(store Store) *PerTurnCaptureHook {
	return NewCaptureHook(NewExtractor(fakeChat{resp: `{"atoms":[{"content":"the sweep archives stale project rows below the threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]}`}),
		captureFakeEmbedder{}, store, 8)
}

func TestCapture_ProcessTurnWritesProjectRow(t *testing.T) {
	store := &captureFakeStore{}
	h := newTestHook(store)
	ex := Exchange{UserText: "how does the GC archive?", AssistantText: "the sweep archives stale project rows below the threshold",
		SubjectID: "userA", SessionID: "s1", ProjectID: "ptolemy"}
	if err := h.processTurn(context.Background(), ex); err != nil {
		t.Fatal(err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("want 1 row, got %d", len(store.upserted))
	}
	c := store.upserted[0]
	if c.Scope != "project" || c.SubjectID == nil || *c.SubjectID != "userA" || c.ProjectID == nil || *c.ProjectID != "ptolemy" {
		t.Fatalf("scoping wrong: %+v", c)
	}
	if c.Importance != factImportance {
		t.Fatalf("factual entry should get factImportance %.2f, got %.2f", factImportance, c.Importance)
	}
	if c.Metadata["extractor_version"] != ExtractorVersion {
		t.Fatalf("extractor_version not stamped: %+v", c.Metadata)
	}
}

func TestCapture_GateSkipsTrivial(t *testing.T) {
	store := &captureFakeStore{}
	h := newTestHook(store)
	_ = h.processTurn(context.Background(), Exchange{UserText: "thanks", AssistantText: "yw", SubjectID: "userA"})
	if len(store.upserted) != 0 {
		t.Fatalf("trivial turn should produce no rows, got %d", len(store.upserted))
	}
}

func TestCapture_EnqueueDoesNotBlockAndDrops(t *testing.T) {
	store := &captureFakeStore{}
	// buffer 1, no worker started → second+ enqueues must drop, never block.
	h := NewCaptureHook(NewExtractor(fakeChat{resp: `{"atoms":[]}`}), captureFakeEmbedder{}, store, 1)
	for i := 0; i < 100; i++ {
		h.Enqueue(Exchange{UserText: "x", AssistantText: "y", SubjectID: "userA"})
	}
	if got := h.Metrics().Dropped(); got == 0 {
		t.Fatalf("expected drops on a full channel, got %d", got)
	}
}

func TestExtractor_EndToEnd_DropsHallucinated(t *testing.T) {
	store := &captureFakeStore{}
	h := NewCaptureHook(NewExtractor(fakeChat{resp: `{"atoms":[{"content":"Quarterly revenue grew forty percent in Brazil","perspective":"factual","fact_subject":"","fact_predicate":"archives"}]}`}), captureFakeEmbedder{}, store, 8)
	ex := Exchange{
		UserText:      "how does the GC archive?",
		AssistantText: "the sweep archives stale project rows below the threshold",
		SubjectID:     "userA", SessionID: "s1", ProjectID: "ptolemy",
	}
	if err := h.processTurn(context.Background(), ex); err != nil {
		t.Fatal(err)
	}
	if len(store.upserted) != 0 {
		t.Fatalf("hallucinated atom should be dropped, got %d stored rows", len(store.upserted))
	}
}

func TestExtractor_EndToEnd_StoresValid(t *testing.T) {
	store := &captureFakeStore{}
	h := NewCaptureHook(NewExtractor(fakeChat{resp: `{"atoms":[{"content":"The GC sweep archives stale project rows below the threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]}`}), captureFakeEmbedder{}, store, 8)
	ex := Exchange{
		UserText:      "how does the GC archive?",
		AssistantText: "The GC sweep archives stale project rows below the threshold",
		SubjectID:     "userA", SessionID: "s1", ProjectID: "ptolemy",
	}
	if err := h.processTurn(context.Background(), ex); err != nil {
		t.Fatal(err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("valid atom should be stored, got %d rows", len(store.upserted))
	}
}

func TestCapture_EmbedCountMismatchErrors(t *testing.T) {
	store := &captureFakeStore{}
	h := NewCaptureHook(NewExtractor(fakeChat{resp: `{"atoms":[{"content":"the sweep archives stale project rows below the threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]}`}), shortVecEmbedder{}, store, 8)
	ex := Exchange{
		UserText:      "how does the GC archive?",
		AssistantText: "the sweep archives stale project rows below the threshold",
		SubjectID:     "userA", SessionID: "s1", ProjectID: "ptolemy",
	}
	if err := h.processTurn(context.Background(), ex); err == nil {
		t.Fatal("expected embed count mismatch error")
	}
}

func TestDeref_NilPointer(t *testing.T) {
	if got := deref(nil); got != "<nil>" {
		t.Fatalf("deref(nil) = %q, want \"<nil>\"", got)
	}
}

func TestDeref_EmptyString(t *testing.T) {
	s := ""
	if got := deref(&s); got != "<nil>" {
		t.Fatalf("deref(&\"\") = %q, want \"<nil>\"", got)
	}
}

func TestDeref_NonEmpty(t *testing.T) {
	s := "project-x"
	if got := deref(&s); got != "project-x" {
		t.Fatalf("deref(&\"project-x\") = %q, want \"project-x\"", got)
	}
}

func TestCapture_ReinforceWhenFactUnchanged(t *testing.T) {
	store := &captureFakeStore{
		lookupFound: true,
		lookupChunk: Chunk{ID: "existing1", Content: "the sweep archives stale project rows below the threshold"},
	}
	h := NewCaptureHook(NewExtractor(fakeChat{resp: `{"atoms":[{"content":"the sweep archives stale project rows below the threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]}`}), captureFakeEmbedder{}, store, 8)
	ex := Exchange{
		UserText:      "how does the GC archive?",
		AssistantText: "the sweep archives stale project rows below the threshold",
		SubjectID:     "userA", SessionID: "s1", ProjectID: "ptolemy",
	}
	if err := h.processTurn(context.Background(), ex); err != nil {
		t.Fatal(err)
	}
	if len(store.reinforced) != 1 || store.reinforced[0] != "existing1" {
		t.Fatalf("expected reinforce on existing unchanged fact, got %+v", store.reinforced)
	}
}
