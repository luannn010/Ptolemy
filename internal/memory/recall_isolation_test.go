package memory

import (
	"context"
	"testing"
	"time"
)

func TestHybrid_SubjectIsolation(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	r := NewHybridRetriever(conn, emb, 0.1, 30*24*time.Hour)

	a, b := "userA", "userB"
	now := time.Now().UTC()
	mk := func(id, content, subj string) Chunk {
		ss, pp, sess, proj := subj, "factual", "s", "ptolemy"
		return Chunk{ID: id, Content: content, Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "project", Importance: 0.7, SubjectID: &ss, SessionID: &sess, ProjectID: &proj, Perspective: &pp}
	}
	if err := s.Upsert(context.Background(), []Chunk{
		mk("pa", "userA secret pancake recipe", a),
		mk("pb", "userB secret pancake recipe", b),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Retrieve(context.Background(), Query{Text: "pancake recipe", K: 10, SubjectID: &a}, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.ID == "pb" {
			t.Fatalf("subject A must not see subject B's row pb; got %d results", len(got))
		}
	}
	// Sanity: A should see its own row (vector arm returns it).
	var sawPA bool
	for _, c := range got {
		if c.ID == "pa" {
			sawPA = true
		}
	}
	if !sawPA {
		t.Fatalf("subject A should recall its own row pa; got %d results", len(got))
	}
}

func TestHybrid_ProjectScope(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	r := NewHybridRetriever(conn, emb, 0.1, 30*24*time.Hour)
	subj := "userA"
	now := time.Now().UTC()
	mk := func(id, content, project string) Chunk {
		ss, pp, sess, pr := subj, "factual", "s", project
		return Chunk{ID: id, Content: content, Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "project", Importance: 0.7, SubjectID: &ss, SessionID: &sess, ProjectID: &pr, Perspective: &pp}
	}
	if err := s.Upsert(context.Background(), []Chunk{
		mk("pa", "alpha config detail one two three", "projA"),
		mk("pb", "alpha config detail one two three", "projB"),
	}); err != nil {
		t.Fatal(err)
	}
	projA := "projA"
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha config", K: 10, SubjectID: &subj, ProjectID: &projA}, 20)
	if err != nil {
		t.Fatal(err)
	}
	var sawA, sawB bool
	for _, c := range got {
		if c.ID == "pa" {
			sawA = true
		}
		if c.ID == "pb" {
			sawB = true
		}
	}
	if !sawA {
		t.Fatalf("project-A query must return projA row; got %v", ids(got))
	}
	if sawB {
		t.Fatalf("project-A query must NOT return projB row; got %v", ids(got))
	}
}
