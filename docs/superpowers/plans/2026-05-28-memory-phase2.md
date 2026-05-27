# Memory Module — Phase 2 (Freshness Layer) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three additive behaviors to the Phase 1 hybrid retrieval pipeline — supersession via explicit `Metadata["supersedes"]`, point-in-time queries via `Query.AsOf`, and a recency boost on the SQL score — without rewriting any existing component.

**Architecture:** Stays inside `internal/memory/`. Adds one migration (`0003_chunks_freshness`) with two indexes; one transactional `Store` method (`SupersedeOnUpsert`); a small branch in `Orchestrator.Ingest` to call it; an `AsOf` default in `Orchestrator.Answer`; and grows the `Bm25Retriever`/`HybridRetriever` SQL in place per `RETRIEVAL.md`'s "the query grows in place" rule. Two files are explicitly NOT touched: `types.go` (`Query.AsOf` already exists from Phase 0) and `rrf_fusion.go` (RRF math unchanged).

**Tech Stack:** Go 1.25; ParadeDB pg_search 0.23.4; `pgx/v5` (we use `pgx.Conn.BeginTx` for the supersession transaction); `pgvector-go`. No new module dependencies.

**Spec:** [docs/superpowers/specs/2026-05-28-memory-phase2.md](../specs/2026-05-28-memory-phase2.md) — locks the four design decisions (explicit supersession, sync at ingest, spec recency defaults, as_of minimum filter) and the three deferrals (RecencyTtlJob, front-matter parsers, eval-set extension).

**Locked decisions (from spec — no need to re-litigate):**
- Supersession detection: explicit only. `RawDocument.Metadata["supersedes"] = "old-doc-id"`.
- Supersession timing: synchronous, inside a Postgres transaction wrapping the upsert.
- Recency: weight `0.1`, half-life `2592000s` (30 days), hard-coded.
- `as_of`: defaults to `time.Now().UTC()` when `Query.AsOf` is nil. SQL filter `published_at <= $5` only — no `valid_from`/`valid_to` window.
- Out of scope: `SupersessionJob`, `RecencyTtlJob`, front-matter parsers, eval-set extension, CLI `--as-of` flag.
- Branch base: `ptolemy/memory-phase2` from clean `main` at `e7c1621` (already created).
- Commit trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` on every commit.
- Stage explicit files only — never `git add .` / `git add -A`.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/migrations/0003_chunks_freshness.sql` | Create | Two `CREATE INDEX IF NOT EXISTS` statements (`chunks_published_at`, partial `chunks_live`). |
| `internal/memory/migrations_test.go` | Modify | Append `TestMigrationsFS_Contains0003` (unit) + `TestApplyMigrations_CreatesFreshnessIndexes` (integration). |
| `internal/memory/store.go` | Modify | Add `SupersedeOnUpsert` to the `Store` interface; implement on `PgStore` using `pgx.Conn.BeginTx`. |
| `internal/memory/store_test.go` | Modify | Append `TestPgStore_SupersedeOnUpsert_HappyPath`, `TestPgStore_SupersedeOnUpsert_NoOldDoc`, `TestPgStore_SupersedeOnUpsert_RollsBackOnError`. |
| `internal/memory/orchestrator.go` | Modify | `Ingest` reads `Metadata["supersedes"]` and dispatches to `SupersedeOnUpsert` vs `Upsert`. `Answer` resolves `q.AsOf` to `time.Now().UTC()` when nil. |
| `internal/memory/orchestrator_test.go` | Modify | Extend `fakeStore` with `supersedeCalls`; add `capturingRetriever`. Append `TestOrchestrator_Ingest_SupersedesOldDoc`, `TestOrchestrator_Ingest_NoSupersedesUsesPlainUpsert`, `TestOrchestrator_Ingest_NonStringSupersedesIsIgnored`, `TestOrchestrator_Answer_AsOfNilDefaultsToNow`, `TestOrchestrator_Answer_AsOfRespected`. |
| `internal/memory/bm25_retriever.go` | Modify | Add `published_at <= $2` to the WHERE; depth becomes `$3`. |
| `internal/memory/bm25_retriever_test.go` | Modify | Append `TestBm25Retriever_PointInTime` (integration). |
| `internal/memory/hybrid_retriever.go` | Modify | `hybridRrfQuery` grows to 5 params: both CTEs get `published_at <= $5`; outer SELECT gets the recency term; outer WHERE mirrors the published_at clause. `Retrieve` passes `q.AsOf` as `$5`. |
| `internal/memory/hybrid_retriever_test.go` | Modify | Append `TestHybridRetriever_PointInTime`, `TestHybridRetriever_PrefersFreshOverStale` (Phase 2 acceptance #1), `TestHybridRetriever_RecencyTermPresent`. |
| `docs/memory/IMPLEMENTATION_PLAN.md` | Modify | Tick Phase 2 + acceptance boxes with implementing-file + test refs (Phase 0/1 style). |

**Two files explicitly NOT touched:**
- `internal/memory/types.go` — `Query.AsOf *time.Time` was added in Phase 0; no struct change needed.
- `internal/memory/rrf_fusion.go` — RRF math unchanged in Phase 2.

**Coverage:** CI runs `go test -coverpkg=./internal/...` against the ParadeDB service container. Every new file has both unit and integration tests; integration tests skip cleanly without `DATABASE_URL` via `requirePG(t)`.

---

## Task 1: Migration `0003_chunks_freshness`

**Files:**
- Create: `internal/memory/migrations/0003_chunks_freshness.sql`
- Modify: `internal/memory/migrations_test.go` (append two new test functions; do not rewrite)

- [ ] **Step 1: Write the failing tests**

Append to `internal/memory/migrations_test.go`:

```go
func TestApplyMigrations_CreatesFreshnessIndexes(t *testing.T) {
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE`)

	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var nPub, nLive int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='chunks' AND indexname='chunks_published_at'`,
	).Scan(&nPub); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='chunks' AND indexname='chunks_live'`,
	).Scan(&nLive); err != nil {
		t.Fatal(err)
	}
	if nPub != 1 {
		t.Fatalf("expected chunks_published_at index, got count=%d", nPub)
	}
	if nLive != 1 {
		t.Fatalf("expected chunks_live partial index, got count=%d", nLive)
	}
}

func TestMigrationsFS_Contains0003(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	want := "0003_chunks_freshness.sql"
	for _, e := range entries {
		if e.Name() == want {
			return
		}
	}
	t.Fatalf("expected %s in embedded migrations", want)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/memory -run 'TestApplyMigrations_CreatesFreshnessIndexes|TestMigrationsFS_Contains0003' -v`

Expected without `DATABASE_URL`:
- `TestMigrationsFS_Contains0003` → FAIL ("expected 0003_chunks_freshness.sql in embedded migrations")
- `TestApplyMigrations_CreatesFreshnessIndexes` → SKIP

With `DATABASE_URL`:
- `TestMigrationsFS_Contains0003` → FAIL (same).
- `TestApplyMigrations_CreatesFreshnessIndexes` → FAIL on index count=0 even if the SQL file is missing because `ApplyMigrations` only runs files it can find — actually skipped if FS check fails first.

- [ ] **Step 3: Create the migration file**

Write `internal/memory/migrations/0003_chunks_freshness.sql`:

```sql
-- Phase 2: support freshness filters cheaply.
-- All freshness columns (published_at, valid_from, valid_to, superseded_by)
-- already exist from 0001_chunks_core.sql; this migration only adds the
-- supporting indexes.
CREATE INDEX IF NOT EXISTS chunks_published_at
    ON chunks (published_at);

CREATE INDEX IF NOT EXISTS chunks_live
    ON chunks (id) WHERE superseded_by IS NULL;
```

The embedded FS picks the file up automatically (per `migrations.go:14`). No code change needed in `migrations.go`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run 'TestApplyMigrations_CreatesFreshnessIndexes|TestMigrationsFS_Contains0003' -v
```

Expected: both PASS. `TestApplyMigrations_CreatesFreshnessIndexes` SKIPs without `DATABASE_URL`; FS test always runs.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/migrations/0003_chunks_freshness.sql internal/memory/migrations_test.go
git commit -m "$(cat <<'EOF'
feat(memory): migration 0003 adds freshness indexes (published_at, partial live)

Why: Phase 2's freshness WHERE clauses (`published_at <= $5`, `superseded_by
IS NULL`) need supporting indexes for the hybrid query to stay fast at the
candidate-depth limit. Both columns already exist from 0001; this migration
only adds the two indexes the spec lists in DATA_MODEL.md §Phase 2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `Store.SupersedeOnUpsert` — interface, PgStore impl, fake Store extension

**Files:**
- Modify: `internal/memory/store.go`
- Modify: `internal/memory/store_test.go` (append three integration tests)
- Modify: `internal/memory/orchestrator_test.go` (extend `fakeStore` so unit tests can satisfy the new interface method)

This task lands the interface AND the production implementation AND the fake-store extension in one commit. Doing the fake extension separately would break the build because the existing orchestrator unit tests would fail to compile against the wider interface.

- [ ] **Step 1: Write the failing integration tests**

Append to `internal/memory/store_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestPgStore_SupersedeOnUpsert -v
```

Expected: compile error — `SupersedeOnUpsert` undefined on `*PgStore`.

- [ ] **Step 3: Extend the `Store` interface and implement `SupersedeOnUpsert` on `PgStore`**

Open `internal/memory/store.go`. Update the `Store` interface (lines 12-16 currently) to:

```go
type Store interface {
	Upsert(ctx context.Context, chunks []Chunk) error
	Get(ctx context.Context, ids []string) ([]Chunk, error)
	MarkSuperseded(ctx context.Context, oldID, newID string) error

	// SupersedeOnUpsert atomically upserts the new chunks AND marks every
	// chunk whose id starts with "<supersedesOldDocID>#" as superseded by the
	// representative new chunk (chunks[0].ID). Both writes are inside one
	// transaction; either both commit or neither does. Calling with a
	// supersedesOldDocID that matches no rows is not an error.
	SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error
}
```

Append the implementation to the bottom of `internal/memory/store.go`:

```go
func (s *PgStore) SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error {
	if len(chunks) == 0 {
		return fmt.Errorf("SupersedeOnUpsert: empty chunk slice")
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Defer Rollback; pgx makes a successful Commit a no-op for Rollback.
	defer func() { _ = tx.Rollback(ctx) }()

	for _, c := range chunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				source = EXCLUDED.source,
				tenant_id = EXCLUDED.tenant_id,
				published_at = EXCLUDED.published_at
		`,
			c.ID, c.Content, pgvector.NewVector(c.Embedding), meta,
			nullableStr(c.Source), nullableStr(c.Tenant), c.PublishedAt,
		)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", c.ID, err)
		}
	}

	// Representative new chunk; the retrieval filter only cares whether
	// superseded_by IS NULL, so a single pointer per supersession is enough.
	rep := chunks[0].ID

	if _, err := tx.Exec(ctx, `
		UPDATE chunks
		SET superseded_by = $1
		WHERE id LIKE $2 || '#%'
		  AND superseded_by IS NULL
	`, rep, supersedesOldDocID); err != nil {
		return fmt.Errorf("supersede %s: %w", supersedesOldDocID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Extend the fake `Store` used by orchestrator unit tests**

The existing `fakeStore` in `internal/memory/orchestrator_test.go:10-21` only implements three methods. With the new interface method, those tests will no longer compile. Modify `fakeStore` to:

```go
type fakeStore struct {
	upserted       []Chunk
	supersedeCalls []supersedeCall
}

type supersedeCall struct {
	OldDocID string
	NewChunks []Chunk
}

func (f *fakeStore) Upsert(_ context.Context, chunks []Chunk) error {
	f.upserted = append(f.upserted, chunks...)
	return nil
}
func (f *fakeStore) Get(_ context.Context, _ []string) ([]Chunk, error) { return nil, nil }
func (f *fakeStore) MarkSuperseded(_ context.Context, _, _ string) error {
	return nil
}
func (f *fakeStore) SupersedeOnUpsert(_ context.Context, chunks []Chunk, oldDocID string) error {
	f.upserted = append(f.upserted, chunks...)
	f.supersedeCalls = append(f.supersedeCalls, supersedeCall{OldDocID: oldDocID, NewChunks: chunks})
	return nil
}
```

This keeps existing orchestrator tests passing (they only check `upserted`) and gives Task 3 a place to assert the dispatch.

- [ ] **Step 5: Run the tests to verify they pass**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -v
```

Expected: all 78+ tests PASS (Phase 1's 75 + the 3 new ones); no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/store.go internal/memory/store_test.go internal/memory/orchestrator_test.go
git commit -m "$(cat <<'EOF'
feat(memory): add Store.SupersedeOnUpsert (transactional)

Why: Phase 2 needs atomic write of new chunks plus retirement of an old
doc's chunks. Single pgx transaction means either both land or neither —
no half-applied supersession on partial failure.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `Orchestrator.Ingest` reads `Metadata["supersedes"]`

**Files:**
- Modify: `internal/memory/orchestrator.go`
- Modify: `internal/memory/orchestrator_test.go` (append three unit tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/memory/orchestrator_test.go`:

```go
func TestOrchestrator_Ingest_SupersedesOldDoc(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID:   "v2",
		Text: "new content",
		Metadata: map[string]any{
			"supersedes": "v1",
		},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.supersedeCalls) != 1 {
		t.Fatalf("expected 1 SupersedeOnUpsert call, got %d", len(store.supersedeCalls))
	}
	if store.supersedeCalls[0].OldDocID != "v1" {
		t.Fatalf("expected OldDocID=v1, got %q", store.supersedeCalls[0].OldDocID)
	}
	if len(store.supersedeCalls[0].NewChunks) != 1 || store.supersedeCalls[0].NewChunks[0].ID != "v2#0" {
		t.Fatalf("expected NewChunks=[v2#0], got %+v", store.supersedeCalls[0].NewChunks)
	}
}

func TestOrchestrator_Ingest_NoSupersedesUsesPlainUpsert(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID:   "d",
		Text: "first ingest",
		// No "supersedes" key.
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.supersedeCalls) != 0 {
		t.Fatalf("expected 0 SupersedeOnUpsert calls, got %d", len(store.supersedeCalls))
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
}

func TestOrchestrator_Ingest_NonStringSupersedesIsIgnored(t *testing.T) {
	// A malformed metadata value (e.g. a number) MUST NOT crash and MUST fall
	// back to plain Upsert. This protects the orchestrator from caller bugs.
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID:   "d",
		Text: "anything",
		Metadata: map[string]any{
			"supersedes": 42, // not a string
		},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.supersedeCalls) != 0 {
		t.Fatalf("non-string supersedes must be ignored; got %d calls", len(store.supersedeCalls))
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 plain upsert, got %d", len(store.upserted))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/memory -run 'TestOrchestrator_Ingest_(SupersedesOldDoc|NoSupersedesUsesPlainUpsert|NonStringSupersedesIsIgnored)' -v
```

Expected: `TestOrchestrator_Ingest_SupersedesOldDoc` FAILs ("expected 1 SupersedeOnUpsert call, got 0") because the production code still calls plain `Upsert`. The other two PASS because they assert the current behavior. After Step 3 all three must PASS.

- [ ] **Step 3: Update `Orchestrator.Ingest` to dispatch**

Read `internal/memory/orchestrator.go`. The current `Ingest` ends with `return o.Store.Upsert(ctx, chunks)`. Replace the final block (lines ~54-58 currently) with:

```go
	for i := range chunks {
		chunks[i].Embedding = vecs[i]
	}
	if old, ok := doc.Metadata["supersedes"].(string); ok && old != "" {
		return o.Store.SupersedeOnUpsert(ctx, chunks, old)
	}
	return o.Store.Upsert(ctx, chunks)
}
```

The type-assert `(string)` returns `("", false)` for any non-string value, which then fails the `ok && old != ""` guard — covering both the missing-key case and the wrong-type case.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/memory -run 'TestOrchestrator_Ingest_(SupersedesOldDoc|NoSupersedesUsesPlainUpsert|NonStringSupersedesIsIgnored)' -v
```

Expected: all 3 PASS.

Also confirm no regressions:

```bash
go test ./internal/memory -v
```

Expected: all tests PASS (existing 78+ plus the 3 new ones).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/orchestrator_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Orchestrator.Ingest dispatches to SupersedeOnUpsert when Metadata["supersedes"] is set

Why: explicit supersession path — caller asserts "this doc replaces X" via
RawDocument.Metadata, orchestrator hands the supersession to the Store's
transactional method. Non-string metadata is ignored (falls back to plain
Upsert) so caller bugs do not crash the ingest path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `Orchestrator.Answer` defaults `q.AsOf` to `time.Now().UTC()` when nil

**Files:**
- Modify: `internal/memory/orchestrator.go`
- Modify: `internal/memory/orchestrator_test.go` (append two unit tests and a `capturingRetriever`)

The retrievers will be updated in Task 5 (Bm25) and Task 6 (Hybrid) to consume `Query.AsOf` as a SQL parameter. This task ensures that by the time a retriever sees a `Query`, `AsOf` is always non-nil.

- [ ] **Step 1: Write the failing tests**

Append to `internal/memory/orchestrator_test.go`:

```go
// capturingRetriever records the last Query it received so tests can assert
// the orchestrator's resolution of optional fields (AsOf, K, etc).
type capturingRetriever struct {
	lastQuery Query
}

func (c *capturingRetriever) Retrieve(_ context.Context, q Query, _ int) ([]RetrievedChunk, error) {
	c.lastQuery = q
	return []RetrievedChunk{
		{Chunk: Chunk{ID: "c1", Content: "anything"}, Score: 0.5},
	}, nil
}

func TestOrchestrator_Answer_AsOfNilDefaultsToNow(t *testing.T) {
	r := &capturingRetriever{}
	o := &Orchestrator{
		Retriever:      r,
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	before := time.Now().UTC().Add(-time.Second)
	if _, err := o.Answer(context.Background(), Query{Text: "q", K: 1}); err != nil {
		t.Fatal(err)
	}
	if r.lastQuery.AsOf == nil {
		t.Fatalf("retriever received Query with AsOf=nil; expected populated default")
	}
	if r.lastQuery.AsOf.Before(before) {
		t.Fatalf("AsOf default %v should be ~now, before=%v", *r.lastQuery.AsOf, before)
	}
}

func TestOrchestrator_Answer_AsOfRespected(t *testing.T) {
	r := &capturingRetriever{}
	o := &Orchestrator{
		Retriever:      r,
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	pin := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := o.Answer(context.Background(), Query{Text: "q", K: 1, AsOf: &pin}); err != nil {
		t.Fatal(err)
	}
	if r.lastQuery.AsOf == nil || !r.lastQuery.AsOf.Equal(pin) {
		t.Fatalf("expected AsOf=%v, got %v", pin, r.lastQuery.AsOf)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/memory -run 'TestOrchestrator_Answer_AsOf' -v
```

Expected: both FAIL — the current `Answer` passes `q` through unchanged, so a nil `AsOf` stays nil.

- [ ] **Step 3: Update `Orchestrator.Answer` to resolve `AsOf`**

Open `internal/memory/orchestrator.go`. The current `Answer` (lines 60-76 today) starts with:

```go
func (o *Orchestrator) Answer(ctx context.Context, q Query) (Answer, error) {
	depth := o.Depth
	if depth <= 0 {
		depth = 20
	}
	candidates, err := o.Retriever.Retrieve(ctx, q, depth)
	...
}
```

Replace it with:

```go
func (o *Orchestrator) Answer(ctx context.Context, q Query) (Answer, error) {
	depth := o.Depth
	if depth <= 0 {
		depth = 20
	}
	// Resolve AsOf default once; downstream retrievers can rely on it being
	// non-nil so they don't each have to default it on their own.
	asOf := time.Now().UTC()
	if q.AsOf != nil {
		asOf = *q.AsOf
	}
	local := q
	local.AsOf = &asOf

	candidates, err := o.Retriever.Retrieve(ctx, local, depth)
	if err != nil {
		return Answer{}, fmt.Errorf("retrieve: %w", err)
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = o.FinalK
	}
	fused := o.Fusion.Fuse([][]RetrievedChunk{candidates}, finalK)
	prompt := o.ContextBuilder.Build(q, fused)
	return o.Generator.Generate(ctx, q, prompt)
}
```

(Keep the rest of the function unchanged. The local variable `local` avoids mutating the caller's Query value.)

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/memory -run 'TestOrchestrator_Answer_AsOf' -v
```

Expected: both PASS.

Full sweep:

```bash
go test ./internal/memory -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/orchestrator_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Orchestrator.Answer resolves Query.AsOf default to now

Why: Phase 2 retrievers (Bm25, Hybrid) take an as_of timestamp as a SQL
parameter. Resolving the default once at the orchestrator boundary means
each retriever sees a non-nil AsOf and does not need its own time.Now()
fallback. Caller's Query is left unmutated.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `Bm25Retriever` adds `published_at <= $2` filter

**Files:**
- Modify: `internal/memory/bm25_retriever.go`
- Modify: `internal/memory/bm25_retriever_test.go` (append `TestBm25Retriever_PointInTime`)

- [ ] **Step 1: Write the failing test**

Append to `internal/memory/bm25_retriever_test.go`:

```go
func TestBm25Retriever_PointInTime(t *testing.T) {
	// Two chunks containing the same rare token, one published before the
	// AsOf and one after. BM25 must surface only the past one.
	conn := freshDB(t)
	s := NewPgStore(conn)
	zero := make([]float32, 4)
	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)

	chunks := []Chunk{
		{ID: "past", Content: "RAREXYZ123 appears here", Embedding: zero, PublishedAt: past},
		{ID: "future", Content: "RAREXYZ123 also lives here", Embedding: zero, PublishedAt: future},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC() // between past and future
	r := NewBm25Retriever(conn)
	got, err := r.Retrieve(context.Background(), Query{Text: "RAREXYZ123", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 || got[0].ID != "past" {
		t.Fatalf("expected only 'past' (published_at <= AsOf), got %+v", idsAndScores(got))
	}
}

func idsAndScores(rs []RetrievedChunk) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = fmt.Sprintf("%s:%.4f", r.ID, r.Score)
	}
	return out
}
```

If `fmt` is not already imported in `bm25_retriever_test.go`, add it. (Spot check before writing the test.)

- [ ] **Step 2: Run the test to verify it fails**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestBm25Retriever_PointInTime -v
```

Expected: FAIL — current SQL does not filter `published_at`; both rows return.

- [ ] **Step 3: Add `published_at <= $2` to the SQL**

Open `internal/memory/bm25_retriever.go`. Locate the SQL constant in `Retrieve` (lines ~30-37 in the current Phase 1 version):

```go
rows, err := r.conn.Query(ctx, `
    SELECT id, content, metadata, COALESCE(source,''), published_at,
           paradedb.score(id) AS score
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
    ORDER BY paradedb.score(id) DESC
    LIMIT $2
`, q.Text, depth)
```

Replace it with:

```go
asOf := time.Now().UTC()
if q.AsOf != nil {
    asOf = *q.AsOf
}
rows, err := r.conn.Query(ctx, `
    SELECT id, content, metadata, COALESCE(source,''), published_at,
           paradedb.score(id) AS score
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
      AND published_at <= $2
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
`, q.Text, asOf, depth)
```

Add the `time` import if it's not already there.

> **Note on the default:** Task 4 made `Orchestrator.Answer` always pass a non-nil `AsOf`, but `Bm25Retriever` is also called by standalone callers (Option B from `ARCHITECTURE.md`) that may not go through the orchestrator. Defaulting locally costs one `if q.AsOf != nil` check; the alternative is to require all callers to set `AsOf` themselves.

- [ ] **Step 4: Run the test to verify it passes**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestBm25Retriever -v
```

Expected: all 4 `TestBm25Retriever_*` tests PASS (the new one plus the three from Phase 1).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/bm25_retriever.go internal/memory/bm25_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Bm25Retriever grows published_at <= $2 filter for point-in-time

Why: Phase 2 acceptance #2 requires that as_of excludes future content from
both retrieval arms. Standalone Bm25Retriever callers (Option B) also get
the filter, with a local Time.Now() fallback so they don't have to pass
AsOf if they don't care about point-in-time semantics.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `HybridRetriever` grows the freshness CTEs + recency term

This is the centerpiece. The `hybridRrfQuery` SQL constant grows from 4 params to 5; both CTEs filter `published_at <= $5`; the outer SELECT adds the recency term to the score. Three new integration tests cover the freshness behavior end-to-end through the Orchestrator pipeline.

**Files:**
- Modify: `internal/memory/hybrid_retriever.go`
- Modify: `internal/memory/hybrid_retriever_test.go` (append three integration tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/memory/hybrid_retriever_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run 'TestHybridRetriever_(PointInTime|PrefersFreshOverStale|RecencyTermPresent)' -v
```

Expected:
- `PointInTime` FAILs — current SQL has no `published_at <= $5` filter.
- `PrefersFreshOverStale` FAILs — supersession works (Tasks 2+3) but the hybrid query still requires Phase 2 grow; will probably PASS once Tasks 2/3 are in… actually since the test only checks that `v1#0` is filtered by `superseded_by IS NULL`, which is already in the Phase 1 SQL, this might PASS even without the freshness changes. Verify by running it: if it PASSes, it's measuring Tasks 2+3 correctly; the harder failure is `PointInTime` and `RecencyTermPresent`.
- `RecencyTermPresent` FAILs — current score has no recency term so the two chunks score equal (the test asserts strictly greater).

- [ ] **Step 3: Update the `hybridRrfQuery` constant and `Retrieve`**

Open `internal/memory/hybrid_retriever.go`. The current `hybridRrfQuery` (lines ~26-50 in Phase 1) ends with `LIMIT $4`. Replace the entire constant with:

```go
const hybridRrfQuery = `
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
      AND published_at <= $5
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE superseded_by IS NULL
      AND published_at <= $5
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0)
     + COALESCE(1.0 / (60 + v.rank), 0)
     + 0.1 * exp(-extract(epoch FROM $5 - c.published_at) / 2592000) AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
  AND c.published_at <= $5
ORDER BY score DESC
LIMIT $4
`
```

Then update `Retrieve` (the existing `r.conn.Query` call) to pass the 5th parameter. The current call is:

```go
rows, err := r.conn.Query(ctx, hybridRrfQuery, q.Text, pgvector.NewVector(vecs[0]), depth, finalK)
```

Replace with:

```go
asOf := time.Now().UTC()
if q.AsOf != nil {
    asOf = *q.AsOf
}
rows, err := r.conn.Query(ctx, hybridRrfQuery, q.Text, pgvector.NewVector(vecs[0]), depth, finalK, asOf)
```

Add the `time` import if it's not already there.

- [ ] **Step 4: Run the new tests to verify they pass**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run 'TestHybridRetriever' -v
```

Expected: all `TestHybridRetriever_*` tests PASS (the Phase 1 ones plus the 3 new ones).

- [ ] **Step 5: Run the full memory test suite to confirm no regressions**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -v
```

Expected: all 80+ tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/hybrid_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): HybridRetriever grows freshness CTEs + recency term

Why: Phase 2's core SQL change — both arms filter published_at <= $5 (point-in-
time semantics, acceptance #2), the outer score adds 0.1*exp(-Δt/30d) so the
recency boost makes fresh content rank slightly above stale content with the
same RRF rank. Constants (0.1, 2592000) are spec defaults; Phase 3 turns them
into tuning knobs against a freshness-aware eval set.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: End-to-end verification — smoke + eval

This is not a TDD task; it's a verification gate before the docs update lands. Both targets must still pass under the new wiring.

**Files:** none (run targets only).

- [ ] **Step 1: Reset the shared dev DB**

(The integration tests in Tasks 2/5/6 left `chunks.embedding` at `vector(4)` via `freshDB(t)`. Reset to clean state so `make smoke-memory` / `make eval-memory` re-migrate at `vector(768)`.)

```bash
PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy \
  -c "DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE;"
```

This requires user authorization (the auto-mode classifier blocks it by default). If denied, ask the user to run it.

- [ ] **Step 2: Build**

```bash
make build
```

Expected: clean build. All five binaries (`workerd`, `ptolemy-mcp`, `policy-demo`, `memory-demo`, `memory-eval`) produced in `bin/`.

- [ ] **Step 3: Smoke test — Phase 0 acceptance still holds**

```bash
make smoke-memory
```

Expected: ingest succeeds; the `ask` step returns a grounded answer with the same kind of citations Phase 1 produced. The smoke doc carries no `Metadata["supersedes"]`, so the new branch in `Orchestrator.Ingest` is not triggered. The recency term gives the just-ingested chunks a small positive boost but does not change ranking against an empty corpus.

If the answer changes shape (e.g. no citations), STOP and investigate before proceeding.

- [ ] **Step 4: Phase 1 eval — acceptance #3 surrogate**

```bash
make eval-memory
```

Expected: `mean recall@5 = 1.000 over 8 questions`. The recency term must not change recall@5 on the evergreen seed (all 7 docs ingested with `published_at = now()`, so their recency contributions are equal).

If the score drops below 1.000, the recency math is wrong — STOP and debug (most likely the `$5 - c.published_at` order, which must yield a non-negative number for chunks published before `asOf`).

- [ ] **Step 5: No commit**

This task produces no code changes. Move directly to Task 8.

---

## Task 8: Tick Phase 2 boxes in `docs/memory/IMPLEMENTATION_PLAN.md`

**Files:**
- Modify: `docs/memory/IMPLEMENTATION_PLAN.md`

- [ ] **Step 1: Edit the Phase 2 section**

In `docs/memory/IMPLEMENTATION_PLAN.md`, replace the existing Phase 2 task list AND acceptance list (currently all `- [ ]`) with the Phase 0/1 style — ticked boxes with implementing-file + test-coverage notes. Use this content verbatim (drop it in place of the existing Phase 2 task block + acceptance block):

```markdown
- [x] Migration `0003_chunks_freshness` (`published_at`, `valid_*`, `superseded_by`,
      indexes).
      → `internal/memory/migrations/0003_chunks_freshness.sql`: adds `chunks_published_at` (btree) and
      `chunks_live` (partial index `id WHERE superseded_by IS NULL`). All freshness columns already
      existed from Phase 0's `0001_chunks_core.sql`. Unit-tested via `TestMigrationsFS_Contains0003`;
      integration test `TestApplyMigrations_CreatesFreshnessIndexes` skips without `DATABASE_URL`.
- [x] Populate `published_at` from real source dates during ingestion.
      → Phase 0 already wires `RawDocument.Metadata["published_at"]` (RFC3339) through
      `Orchestrator.Ingest` to `ParsedDocument.PublishedAt`, and `FixedSizeChunker` propagates it to
      every chunk. No code change in Phase 2; existing test
      `TestOrchestrator_IngestSetsPublishedAtFromSource` covers it.
- [x] Add freshness `WHERE` clauses + recency term to the query (`RETRIEVAL.md` Phase 2).
      → `internal/memory/hybrid_retriever.go`: `hybridRrfQuery` grew from 4 to 5 params. Both CTEs
      and the outer SELECT carry `published_at <= $5`. Outer score adds
      `0.1 * exp(-extract(epoch FROM $5 - c.published_at) / 2592000)`. `internal/memory/bm25_retriever.go`
      symmetrically grows from 2 to 3 params. Integration tests `TestHybridRetriever_PointInTime`,
      `TestHybridRetriever_RecencyTermPresent`, `TestBm25Retriever_PointInTime` cover both arms.
- [x] `SupersessionJob` — detect replacements, set `superseded_by`. (Detection strategy:
      resolve the open question below first.)
      → Resolved during Phase 2 brainstorming: **explicit document versioning** via
      `RawDocument.Metadata["supersedes"]`. Performed **synchronously at ingest** inside a Postgres
      transaction by `PgStore.SupersedeOnUpsert` (no separate async job needed). The spec's heavy
      `SupersessionJob` was sized for the rejected embedding-similarity strategy.
      `Orchestrator.Ingest` reads the metadata and dispatches; non-string values are silently
      ignored. Unit tests: `TestOrchestrator_Ingest_SupersedesOldDoc`,
      `TestOrchestrator_Ingest_NoSupersedesUsesPlainUpsert`,
      `TestOrchestrator_Ingest_NonStringSupersedesIsIgnored`. Integration tests:
      `TestPgStore_SupersedeOnUpsert_HappyPath`, `TestPgStore_SupersedeOnUpsert_NoOldDoc`,
      `TestPgStore_SupersedeOnUpsert_RollsBackOnError`.
- [x] `RecencyTtlJob` — refresh/expire policy.
      → **Deferred to Phase 3** per the brainstorming decision. Ptolemy's content is evergreen
      reference docs with no concrete TTL policy yet; the spec's Phase 3 says "add enhancements
      one at a time, measure each." Pulled when there is a corpus that benefits from it.
- [x] Expose `as_of` on the `Query` type; default to `now()`.
      → `Query.AsOf *time.Time` was added in Phase 0; Phase 2 wires it. `Orchestrator.Answer`
      resolves the default to `time.Now().UTC()` once at the boundary
      (`TestOrchestrator_Answer_AsOfNilDefaultsToNow`,
      `TestOrchestrator_Answer_AsOfRespected`). Both retrievers consume the value as a SQL
      parameter.

**Acceptance:**
- [x] Given an old and a corrected chunk for the same fact, retrieval returns the
      corrected one and not the stale one.
      → `TestHybridRetriever_PrefersFreshOverStale` exercises the full path:
      `Orchestrator.Ingest` of v1, then ingest of v2 with `Metadata["supersedes"]="v1"`, then a
      query; the v2 chunks are returned and v1 is absent (filtered by `superseded_by IS NULL`).
- [x] A point-in-time query with a past `as_of` excludes content published after it.
      → `TestHybridRetriever_PointInTime` + `TestBm25Retriever_PointInTime`: insert chunks at
      past and future timestamps, query with `AsOf` between them, assert only the past chunk
      returns. Both retrieval arms covered.
- [x] Recency weighting measurably raises fresh results without tanking eval-set score.
      → `TestHybridRetriever_RecencyTermPresent` proves the recency term is non-zero and
      monotonic: two chunks identical except for `published_at` (1h vs 60d old) produce
      strictly different scores in the expected direction. Eval-set surrogate:
      `make eval-memory` still scores `mean recall@5 = 1.000 over 8 questions` after Phase 2,
      so the recency term did not degrade evergreen recall. Direct fresh-vs-stale eval
      questions land in Phase 3 alongside the recency-constant tuning.
```

- [ ] **Step 2: Commit**

```bash
git add docs/memory/IMPLEMENTATION_PLAN.md
git commit -m "$(cat <<'EOF'
docs(memory): tick Phase 2 + acceptance checkboxes with file/test notes

Why: Phase 2 implementation landed across commits for migration 0003,
Store.SupersedeOnUpsert, Orchestrator dispatch, Orchestrator.AsOf default,
Bm25 published_at filter, HybridRetriever freshness CTEs + recency term.
Plan file should reflect what was actually built and how it was verified,
matching the Phase 0/1 record-keeping style.

Recorded scores: make eval-memory still mean recall@5 = 1.000 after the
recency-term addition (acceptance #3 surrogate).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Push branch + open the PR

Per AGENTS.md: no `gh` CLI; surface the PR URL to the user for web-UI submission.

- [ ] **Step 1: Push the branch**

```bash
git push -u origin ptolemy/memory-phase2
```

Expected: branch pushed, GitHub returns the create-PR URL on the second-to-last line.

- [ ] **Step 2: Surface the PR materials to the user**

Print the create-PR URL + the suggested title + body. Title:

```
feat(memory): Phase 2 — freshness layer (explicit supersession + as_of + recency boost)
```

Body template:

```markdown
## Summary

- Adds migration `0003_chunks_freshness` with two supporting indexes (`chunks_published_at`, partial `chunks_live`).
- Adds `Store.SupersedeOnUpsert` — a transactional upsert+supersede that runs inside a single pgx transaction.
- `Orchestrator.Ingest` dispatches to `SupersedeOnUpsert` when `RawDocument.Metadata["supersedes"]` is set (non-string values are silently ignored).
- `Orchestrator.Answer` resolves `Query.AsOf` to `time.Now().UTC()` once at the boundary so retrievers see a non-nil value.
- `Bm25Retriever` and `HybridRetriever` SQL grow in place: both gain `published_at <= $N`; `HybridRetriever`'s outer score adds `0.1 * exp(-Δt/30d)`.
- Two files explicitly NOT touched: `internal/memory/types.go` (`Query.AsOf` already there from Phase 0), `internal/memory/rrf_fusion.go` (RRF math unchanged).

## Acceptance (measured against live ParadeDB + nomic-embed + Qwen3.5-4B)

- (1) Supersession: `TestHybridRetriever_PrefersFreshOverStale` integration test — full Orchestrator path.
- (2) Point-in-time: `TestHybridRetriever_PointInTime` + `TestBm25Retriever_PointInTime`.
- (3) Recency-term + eval surrogate: `TestHybridRetriever_RecencyTermPresent` proves the term is non-zero and monotonic; `make eval-memory` still scores `mean recall@5 = 1.000 over 8 questions` (recency did not degrade evergreen recall).

## Test plan

- [x] `go test -coverpkg=./internal/... ./...` against the ParadeDB service container in CI.
- [x] `make smoke-memory` still answers "What is Ptolemy?" with grounded citations.
- [x] `make eval-memory` still prints `mean recall@5 = 1.000 over 8 questions`.
- [x] Local integration tests against `192.168.0.164:1091`:
      `TestApplyMigrations_CreatesFreshnessIndexes`,
      `TestPgStore_SupersedeOnUpsert_HappyPath`,
      `TestPgStore_SupersedeOnUpsert_RollsBackOnError`,
      `TestPgStore_SupersedeOnUpsert_NoOldDoc`,
      `TestHybridRetriever_PointInTime`, `TestHybridRetriever_PrefersFreshOverStale`,
      `TestHybridRetriever_RecencyTermPresent`,
      `TestBm25Retriever_PointInTime`.

## Out of scope (deferred to Phase 3)

- `RecencyTtlJob`, front-matter parsers, valid_from/valid_to window filter, CLI `--as-of` flag, eval-set freshness questions, recency-constant tuning.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

Print the URL `https://github.com/luannn010/Ptolemy/pull/new/ptolemy/memory-phase2` and wait for the user to merge via the web UI.

---

## Acceptance recap (Phase 2)

Done when ALL of the following are true:

1. `go test ./...` is green locally and in CI (coverage gate ≥ 80%).
2. `make smoke-memory` still answers a question with grounded citations.
3. `make eval-memory` still prints `mean recall@5 = 1.000 over 8 questions` (recency did not degrade evergreen recall).
4. The three acceptance tests above all PASS against the live ParadeDB.
5. Phase 2 rows in `IMPLEMENTATION_PLAN.md` are ticked with file+test notes (Phase 0/1 style).
6. PR opened from `ptolemy/memory-phase2` → `main`, URL surfaced to user.

---

## Self-review

**Spec coverage:** every spec requirement maps to a task.
- Migration → Task 1.
- Supersession (Store + Orchestrator + tests) → Tasks 2 + 3.
- `as_of` default → Task 4.
- Bm25 grow → Task 5.
- Hybrid grow + recency term → Task 6.
- Smoke + eval verification → Task 7.
- Docs update → Task 8.
- PR → Task 9.

**Placeholder scan:** no `TBD`, no "add appropriate error handling", no "similar to Task N". Every step has the literal code or command an engineer needs.

**Type consistency:**
- `SupersedeOnUpsert(ctx, chunks []Chunk, supersedesOldDocID string) error` — referenced consistently across the `Store` interface, `PgStore` implementation, `fakeStore` test double, orchestrator dispatch, and the commit messages.
- `Query.AsOf *time.Time` — used consistently; nil handled in `Orchestrator.Answer` (Task 4) and defaulted to `time.Now().UTC()` in both retrievers' local-fallback paths (Tasks 5 + 6).
- Representative chunk `chunks[0].ID` — referenced in the spec, the `PgStore.SupersedeOnUpsert` doc comment, the `TestPgStore_SupersedeOnUpsert_HappyPath` assertions, and the IMPLEMENTATION_PLAN.md fill-in. All match.
- `hybridRrfQuery` parameter order — `$1` text, `$2` vector, `$3` depth, `$4` final-k, `$5` as_of. Same in the SQL constant, the `r.conn.Query` call, the doc comment, and the IMPLEMENTATION_PLAN.md write-up.
