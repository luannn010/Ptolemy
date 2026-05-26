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
