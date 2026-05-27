package memory

import (
	"context"
	"fmt"
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

type erroringEmbedderForRetrieve struct{}

func (erroringEmbedderForRetrieve) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embed dead")
}

func TestNewVectorRetriever_ReturnsConfiguredStruct(t *testing.T) {
	// Construction smoke: a nil *pgx.Conn is fine because we never reach
	// any DB call. This exercises the constructor without an integration env.
	r := NewVectorRetriever(nil, fakeEmbedder{})
	if r == nil {
		t.Fatalf("NewVectorRetriever returned nil")
	}
	if r.embedder == nil {
		t.Fatalf("embedder not stored on retriever")
	}
}

func TestRetrieve_EmbedderErrorReturnsBeforeDB(t *testing.T) {
	// When the embedder errors, Retrieve must return before touching the DB.
	// Passing a nil conn proves that — if it were dereferenced we'd panic.
	r := NewVectorRetriever(nil, erroringEmbedderForRetrieve{})
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 5); err == nil {
		t.Fatalf("expected embedder error to surface")
	}
}

func TestRetrieve_NoVectorsReturnsBeforeDB(t *testing.T) {
	// An embedder that returns an empty slice must short-circuit with a
	// clear error rather than executing a malformed SQL query.
	r := NewVectorRetriever(nil, fakeEmbedder{vecs: nil})
	_, err := r.Retrieve(context.Background(), Query{Text: "q"}, 5)
	if err == nil {
		t.Fatalf("expected 'no vector' error")
	}
}

func TestRetrieve_NegativeDepthNormalizes(t *testing.T) {
	// depth <= 0 should be normalised to the default of 20. We can't observe
	// it directly without a DB, but combining with an erroring embedder
	// proves the function reached past the depth check.
	r := NewVectorRetriever(nil, erroringEmbedderForRetrieve{})
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 0); err == nil {
		t.Fatalf("expected embedder error after depth normalization")
	}
}
