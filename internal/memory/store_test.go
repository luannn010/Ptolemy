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
	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE`)
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

func TestPgStore_Supersede_HidesOldShowsNew(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	if err := s.Upsert(ctx, []Chunk{
		{ID: "f1", Content: "deploy target is AWS", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "global", FactSubject: ptr("deploy"), FactPredicate: ptr("target")},
	}); err != nil {
		t.Fatal(err)
	}
	newChunk := Chunk{ID: "f2", Content: "deploy target is GCP", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now,
		Scope: "global", FactSubject: ptr("deploy"), FactPredicate: ptr("target")}
	if err := s.Supersede(ctx, []Chunk{newChunk}, "f1"); err != nil {
		t.Fatalf("Supersede: %v", err)
	}

	got, err := s.Get(ctx, []string{"f1", "f2"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Chunk{}
	for _, c := range got {
		byID[c.ID] = c
	}
	if byID["f1"].Status != "superseded" {
		t.Errorf("old f1 status=%q, want superseded", byID["f1"].Status)
	}
	if byID["f1"].SupersededBy == nil || *byID["f1"].SupersededBy != "f2" {
		t.Errorf("old f1 superseded_by=%v, want f2", byID["f1"].SupersededBy)
	}
	if byID["f2"].Status != "active" {
		t.Errorf("new f2 status=%q, want active", byID["f2"].Status)
	}
	if byID["f2"].Supersedes == nil || *byID["f2"].Supersedes != "f1" {
		t.Errorf("new f2 supersedes=%v, want f1", byID["f2"].Supersedes)
	}
	if byID["f2"].Version != 2 {
		t.Errorf("new f2 version=%d, want 2", byID["f2"].Version)
	}

	var auditReason string
	if err := conn.QueryRow(ctx,
		`SELECT reason FROM chunk_audit WHERE chunk_id='f1' AND new_status='superseded'`).Scan(&auditReason); err != nil {
		t.Fatalf("expected audit row for f1: %v", err)
	}
}

func TestPgStore_History_WalksChain(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	if err := s.Upsert(ctx, []Chunk{{ID: "v1", Content: "one", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "global"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Supersede(ctx, []Chunk{{ID: "v2", Content: "two", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "global"}}, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Supersede(ctx, []Chunk{{ID: "v3", Content: "three", Embedding: []float32{0, 0, 1, 0}, PublishedAt: now, Scope: "global"}}, "v2"); err != nil {
		t.Fatal(err)
	}
	hist, err := s.History(ctx, "v3")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("History len=%d, want 3", len(hist))
	}
	if hist[0].ID != "v1" || hist[1].ID != "v2" || hist[2].ID != "v3" {
		t.Errorf("History order = %s,%s,%s, want v1,v2,v3", hist[0].ID, hist[1].ID, hist[2].ID)
	}
}

func TestPgStore_Supersede_Reversible(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	_ = s.Upsert(ctx, []Chunk{{ID: "r1", Content: "old", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "global"}})
	_ = s.Supersede(ctx, []Chunk{{ID: "r2", Content: "new", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "global"}}, "r1")
	// Rollback is a single UPDATE.
	if _, err := conn.Exec(ctx, `UPDATE chunks SET status='active', superseded_by=NULL WHERE id='r1'`); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, []string{"r1"})
	if len(got) != 1 || got[0].Status != "active" {
		t.Fatalf("r1 not restored to active: %+v", got)
	}
}

func TestPgStore_SupersedeOnUpsert_StampsUnifiedModel(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	// Old doc has two chunks under the "doc1#" id prefix.
	if err := s.Upsert(ctx, []Chunk{
		{ID: "doc1#0", Content: "old a", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "global"},
		{ID: "doc1#1", Content: "old b", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	newChunks := []Chunk{{ID: "doc2#0", Content: "new a", Embedding: []float32{0, 0, 1, 0}, PublishedAt: now, Scope: "global"}}
	if err := s.SupersedeOnUpsert(ctx, newChunks, "doc1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, []string{"doc1#0", "doc1#1"})
	for _, c := range got {
		if c.Status != "superseded" {
			t.Errorf("%s status=%q, want superseded", c.ID, c.Status)
		}
		if c.SupersededBy == nil || *c.SupersededBy != "doc2#0" {
			t.Errorf("%s superseded_by=%v, want doc2#0", c.ID, c.SupersededBy)
		}
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

func TestPgStore_Reinforce_BumpsCounters(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "r1", Content: "x", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	_ = conn.QueryRow(context.Background(),
		`SELECT last_accessed_at FROM chunks WHERE id='r1'`).Scan(&before)

	if err := s.Reinforce(context.Background(), []string{"r1"}); err != nil {
		t.Fatalf("Reinforce: %v", err)
	}

	var count int
	var after time.Time
	if err := conn.QueryRow(context.Background(),
		`SELECT access_count, last_accessed_at FROM chunks WHERE id='r1'`).Scan(&count, &after); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("access_count: got %d want 1", count)
	}
	if !after.After(before) {
		t.Fatalf("last_accessed_at not advanced: before=%v after=%v", before, after)
	}
}

func TestPgStore_Reinforce_EmptyIsNoOp(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	if err := s.Reinforce(context.Background(), nil); err != nil {
		t.Fatalf("empty Reinforce should be a no-op, got %v", err)
	}
}

func TestPgStore_Stats_CountsByScopeStatus(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "g1", Content: "a", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
		{ID: "g2", Content: "b", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	// Force a project+archived row directly.
	_, err := conn.Exec(context.Background(),
		`UPDATE chunks SET scope='project', status='archived' WHERE id='g2'`)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	got := map[string]int{}
	for _, sc := range stats {
		got[sc.Scope+"/"+sc.Status] = sc.Count
	}
	if got["global/active"] != 1 || got["project/archived"] != 1 {
		t.Fatalf("stats wrong: %+v", got)
	}
}

func TestPgStore_Supersede_UnknownOldIDErrors(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	err := s.Supersede(ctx, []Chunk{{ID: "x", Content: "c", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(), Scope: "global"}}, "nonexistent")
	if err == nil {
		t.Fatal("expected error superseding a non-existent oldID")
	}
	got, _ := s.Get(ctx, []string{"x"})
	if len(got) != 0 {
		t.Fatalf("expected rollback; row x should not exist, got %+v", got)
	}
}

func ptr(s string) *string { return &s }
