package memory

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func freshDB(t *testing.T) *pgx.Conn {
	t.Helper()
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE`)
	if err := ApplyMigrations(context.Background(), conn, 4); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

func TestPgStore_UpsertAndGet(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	chunks := []Chunk{{
		ID:          "c1",
		Content:     "hello world",
		Embedding:   []float32{1, 0, 0, 0},
		Metadata:    map[string]any{"k": "v"},
		Source:      "test",
		PublishedAt: time.Now().UTC(),
	}}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.Get(context.Background(), []string{"c1"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 || got[0].Content != "hello world" {
		t.Fatalf("unexpected get result: %+v", got)
	}
	if got[0].Metadata["k"] != "v" {
		t.Fatalf("metadata not round-tripped: %+v", got[0].Metadata)
	}
}

func TestPgStore_MarkSuperseded(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()
	chunks := []Chunk{
		{ID: "a", Content: "old", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now.Add(-time.Hour)},
		{ID: "b", Content: "new", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSuperseded(context.Background(), "a", "b"); err != nil {
		t.Fatalf("MarkSuperseded: %v", err)
	}
	got, _ := s.Get(context.Background(), []string{"a"})
	if got[0].SupersededBy == nil || *got[0].SupersededBy != "b" {
		t.Fatalf("expected SupersededBy=b, got %+v", got[0].SupersededBy)
	}
}

func TestNullableStr(t *testing.T) {
	if got := nullableStr(""); got != nil {
		t.Fatalf("nullableStr(\"\") must be nil, got %v", got)
	}
	if got := nullableStr("x"); got != "x" {
		t.Fatalf("nullableStr(\"x\") must be \"x\", got %v", got)
	}
	if got := nullableStr("  "); got != "  " {
		t.Fatalf("nullableStr preserves non-empty whitespace, got %v", got)
	}
}

func TestPgStore_SupersedeOnUpsert_HappyPath(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()

	// Seed v1 with 3 chunks.
	v1 := []Chunk{
		{ID: "v1#0", Content: "old part 0", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now.Add(-2 * time.Hour)},
		{ID: "v1#1", Content: "old part 1", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now.Add(-2 * time.Hour)},
		{ID: "v1#2", Content: "old part 2", Embedding: []float32{0, 0, 1, 0}, PublishedAt: now.Add(-2 * time.Hour)},
	}
	if err := s.Upsert(context.Background(), v1); err != nil {
		t.Fatal(err)
	}

	v2 := []Chunk{
		{ID: "v2#0", Content: "new part 0", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
		{ID: "v2#1", Content: "new part 1", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now},
	}
	if err := s.SupersedeOnUpsert(context.Background(), v2, "v1"); err != nil {
		t.Fatalf("SupersedeOnUpsert: %v", err)
	}

	// All three old chunks must now be superseded by the representative new chunk v2#0.
	got, err := s.Get(context.Background(), []string{"v1#0", "v1#1", "v1#2", "v2#0", "v2#1"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Chunk{}
	for _, c := range got {
		byID[c.ID] = c
	}
	for _, old := range []string{"v1#0", "v1#1", "v1#2"} {
		c := byID[old]
		if c.SupersededBy == nil || *c.SupersededBy != "v2#0" {
			t.Fatalf("%s expected superseded_by=v2#0, got %+v", old, c.SupersededBy)
		}
	}
	for _, fresh := range []string{"v2#0", "v2#1"} {
		c := byID[fresh]
		if c.SupersededBy != nil {
			t.Fatalf("%s should be live, got superseded_by=%v", fresh, *c.SupersededBy)
		}
	}
}

func TestPgStore_SupersedeOnUpsert_NoOldDoc(t *testing.T) {
	// Referencing a doc id that has no chunks must succeed (0 rows updated)
	// and still upsert the new chunks. Logged at info level by the caller,
	// but the Store itself is permissive.
	conn := freshDB(t)
	s := NewPgStore(conn)

	new := []Chunk{
		{ID: "new#0", Content: "fresh", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
	}
	if err := s.SupersedeOnUpsert(context.Background(), new, "nonexistent"); err != nil {
		t.Fatalf("expected no error for missing old doc, got %v", err)
	}
	got, _ := s.Get(context.Background(), []string{"new#0"})
	if len(got) != 1 {
		t.Fatalf("new chunk should still have been upserted, got %d", len(got))
	}
}

func TestPgStore_SupersedeOnUpsert_RollsBackOnError(t *testing.T) {
	// Force a referential-integrity failure on the upsert step by providing a
	// chunk with a content NULL violation (Content is NOT NULL in the schema).
	// The supersede UPDATE must not commit.
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()

	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "v1#0", Content: "old", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now.Add(-time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	// pgvector rejects mismatched dim — chunks_core was created with dim=4
	// (see freshDB), so a 3-element vector triggers a runtime SQL error inside
	// the transaction.
	bad := []Chunk{
		{ID: "v2#0", Content: "new", Embedding: []float32{1, 0, 0}, PublishedAt: now},
	}
	if err := s.SupersedeOnUpsert(context.Background(), bad, "v1"); err == nil {
		t.Fatalf("expected dim-mismatch error, got nil")
	}

	// Old chunk must still be live (rollback worked).
	got, _ := s.Get(context.Background(), []string{"v1#0", "v2#0"})
	byID := map[string]Chunk{}
	for _, c := range got {
		byID[c.ID] = c
	}
	if byID["v1#0"].SupersededBy != nil {
		t.Fatalf("v1#0 should still be live after rollback, got superseded_by=%v", *byID["v1#0"].SupersededBy)
	}
	if _, ok := byID["v2#0"]; ok {
		t.Fatalf("v2#0 must not exist after rollback, got %+v", byID["v2#0"])
	}
}
