package memory

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewHybridRetriever_ReturnsConfiguredStruct(t *testing.T) {
	r := NewHybridRetriever(nil, fakeEmbedder{})
	if r == nil {
		t.Fatalf("NewHybridRetriever returned nil")
	}
}

func TestHybridRetriever_EmbedderErrorReturnsBeforeDB(t *testing.T) {
	r := NewHybridRetriever(nil, erroringEmbedderForRetrieve{})
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 5); err == nil {
		t.Fatalf("expected embedder error to surface")
	}
}

func TestHybridRetriever_NoVectorsReturnsBeforeDB(t *testing.T) {
	r := NewHybridRetriever(nil, fakeEmbedder{vecs: nil})
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 5); err == nil {
		t.Fatalf("expected 'no vector' error")
	}
}

func TestHybridRetriever_NegativeDepthNormalizes(t *testing.T) {
	r := NewHybridRetriever(nil, erroringEmbedderForRetrieve{})
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 0); err == nil {
		t.Fatalf("expected embedder error after depth normalization")
	}
}

func TestHybridRetriever_ExactTokenWins(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	chunks := []Chunk{
		{ID: "near", Content: "alpha", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
		{ID: "exact", Content: "RAREXYZ123 is the unique token", Embedding: []float32{0, 0, 0, 1}, PublishedAt: time.Now().UTC()},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}})
	got, err := r.Retrieve(context.Background(), Query{Text: "RAREXYZ123", K: 5}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	seen := map[string]bool{}
	for _, rc := range got {
		seen[rc.ID] = true
	}
	if !seen["exact"] {
		t.Fatalf("expected hybrid to include BM25 exact match 'exact', got %v", idsHybrid(got))
	}
	if !seen["near"] {
		t.Fatalf("expected hybrid to include vector-close 'near', got %v", idsHybrid(got))
	}
}

func idsHybrid(rs []RetrievedChunk) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = fmt.Sprintf("%s:%.4f", r.ID, r.Score)
	}
	return out
}

func TestHybridRetriever_PointInTime(t *testing.T) {
	// Acceptance #2: a query with a past AsOf excludes content published after it.
	conn := freshDB(t)
	s := NewPgStore(conn)
	past := time.Now().UTC().Add(-72 * time.Hour)
	future := time.Now().UTC().Add(72 * time.Hour)

	chunks := []Chunk{
		{ID: "past", Content: "alpha bravo charlie", Embedding: []float32{1, 0, 0, 0}, PublishedAt: past},
		{ID: "future", Content: "alpha bravo charlie", Embedding: []float32{1, 0, 0, 0}, PublishedAt: future},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC()
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}})
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 || got[0].ID != "past" {
		t.Fatalf("expected only 'past' (published_at <= AsOf), got %+v", idsHybrid(got))
	}
}

func TestHybridRetriever_PrefersFreshOverStale(t *testing.T) {
	// Acceptance #1: ingest v1, then ingest v2 with Metadata[supersedes]=v1
	// through the full Orchestrator path; query → v2 returned, v1 absent.
	conn := freshDB(t)
	s := NewPgStore(conn)
	embedder := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: embedder,
		Store:    s,
	}

	// Ingest v1 (no supersedes).
	if err := o.Ingest(context.Background(), RawDocument{
		ID:   "v1",
		Text: "alpha bravo",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest v2 declaring it supersedes v1.
	if err := o.Ingest(context.Background(), RawDocument{
		ID:   "v2",
		Text: "alpha bravo",
		Metadata: map[string]any{
			"supersedes": "v1",
		},
	}); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC().Add(time.Hour) // ensure both <= asOf
	r := NewHybridRetriever(conn, embedder)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, rc := range got {
		if rc.ID == "v1#0" {
			t.Fatalf("v1#0 must be filtered by superseded_by IS NULL; got %+v", idsHybrid(got))
		}
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one result containing v2 chunks; got nothing")
	}
}

func TestHybridRetriever_RecencyTermPresent(t *testing.T) {
	// Two chunks with identical text + identical embeddings but different
	// published_at. The fresher chunk's Score must be strictly greater than
	// the older one's because the recency term is positive and monotonic.
	conn := freshDB(t)
	s := NewPgStore(conn)
	fresh := time.Now().UTC().Add(-1 * time.Hour)
	stale := time.Now().UTC().Add(-60 * 24 * time.Hour) // 60 days old

	chunks := []Chunk{
		{ID: "fresh", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: fresh},
		{ID: "stale", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: stale},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC()
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}})
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(got), idsHybrid(got))
	}
	scoreByID := map[string]float64{}
	for _, rc := range got {
		scoreByID[rc.ID] = rc.Score
	}
	if scoreByID["fresh"] <= scoreByID["stale"] {
		t.Fatalf("expected fresh.Score > stale.Score, got fresh=%v stale=%v",
			scoreByID["fresh"], scoreByID["stale"])
	}
}
