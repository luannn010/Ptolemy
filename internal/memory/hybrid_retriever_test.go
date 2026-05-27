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
