package memory

import (
	"context"
	"testing"
	"time"
)

func TestNewBm25Retriever_ReturnsConfiguredStruct(t *testing.T) {
	// Construction smoke without a real DB: nil conn is fine because no SQL runs.
	r := NewBm25Retriever(nil)
	if r == nil {
		t.Fatalf("NewBm25Retriever returned nil")
	}
}

func TestBm25Retriever_EmptyQueryReturnsEmpty(t *testing.T) {
	// An empty query string would degenerate to "match nothing" in pg_search.
	// Short-circuit to nil before hitting the DB; passing nil conn proves it.
	r := NewBm25Retriever(nil)
	got, err := r.Retrieve(context.Background(), Query{Text: "", K: 5}, 5)
	if err != nil {
		t.Fatalf("expected nil err on empty query, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result on empty query, got %d", len(got))
	}
}

func TestBm25Retriever_FindsExactToken(t *testing.T) {
	// Integration: BM25 must surface a chunk containing a rare exact token
	// even when other chunks share semantic content. We don't compute
	// embeddings here — store.Upsert requires a non-nil vector, so we provide
	// a placeholder of the correct dimension that's never used by BM25.
	conn := freshDB(t)
	s := NewPgStore(conn)
	zero := make([]float32, 4)

	chunks := []Chunk{
		{ID: "hit", Content: "the unique token RAREXYZ123 lives here", Embedding: zero, PublishedAt: time.Now().UTC()},
		{ID: "miss", Content: "completely unrelated other content", Embedding: zero, PublishedAt: time.Now().UTC()},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	r := NewBm25Retriever(conn)
	got, err := r.Retrieve(context.Background(), Query{Text: "RAREXYZ123", K: 5}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) < 1 || got[0].ID != "hit" {
		t.Fatalf("expected 'hit' to be top BM25 result, got %+v", got)
	}
}
