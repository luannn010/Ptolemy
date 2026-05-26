package memory

import (
	"context"
	"testing"
	"time"
)

func TestVectorRetriever_FindsNearest(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	chunks := []Chunk{
		{ID: "near", Content: "alpha", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
		{ID: "far", Content: "beta", Embedding: []float32{0, 0, 0, 1}, PublishedAt: time.Now().UTC()},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	embedder := fakeEmbedder{[][]float32{{1, 0, 0, 0}}}
	r := NewVectorRetriever(conn, embedder)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) < 1 || got[0].ID != "near" {
		t.Fatalf("expected 'near' to be top result, got %+v", got)
	}
}

type fakeEmbedder struct{ vecs [][]float32 }

func (f fakeEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return f.vecs, nil
}
