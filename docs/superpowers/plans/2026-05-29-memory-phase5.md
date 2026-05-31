# Memory Phase 5 (GC Supersession + Dedup) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the version chain the single supersession path, add the zero-cost structured-fact ladder + `confidence` flow at ingest, and add a scope-gated trigram `dedupRecent()` sweep pass (shipped but gated off) — all pure-DB, no embeddings/LLM in the comparison path.

**Architecture:** Additive to the Phase 4 lifecycle. One migration (`0005`) enables `pg_trgm` + two indexes + a status backfill; `store.go` gains a single transactional supersession helper used by both `Supersede()` (row-level) and the reworked `SupersedeOnUpsert()` (doc-level); retrieval collapses to a single `status='active'` filter; `Orchestrator.Ingest` runs the structured-fact ladder; the sweep gains a gated `dedupRecent()` whose collapse bar is normalized-content *equality* among trigram candidates (so a contradiction is never collapsed).

**Tech Stack:** Go 1.25, `github.com/jackc/pgx/v5`, `github.com/pgvector/pgvector-go`, PostgreSQL + ParadeDB (pg_search BM25, pgvector) + `pg_trgm` 1.6, `zerolog`. Module path `github.com/luannn010/ptolemy`.

---

## Conventions for every task

- **TDD, strictly:** write the failing test → run it red → minimal implementation → run it green → commit. One logical change per commit.
- **Run unit tests:** `go test ./internal/memory/ -run <TestName> -v`. **Run the full suite:** `make test` (uses `-p 1`).
- **Integration tests** use the existing `freshDB(t)` helper (in `internal/memory/store_test.go`): it `DROP`s `chunks, chunk_audit, memory_schema_migrations`, re-applies all migrations with `dim=4`, and returns a `*pgx.Conn`. `freshDB` calls `requirePG(t)`, which **skips** the test when `DATABASE_URL` is unset. To actually exercise them: `DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test ./internal/memory/ -run <TestName> -v`.
- **Commits:** explicit `git add <paths>` only (never `git add .`/`-A`). Message trailer:
  ```
  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```
- **No `gh` CLI.** The PR is surfaced for web-UI submission at the end.
- **Branch:** all work on `ptolemy/memory-phase5`, cut from clean `main` after the Phase 4 PR merges (Task 1).

---

## Task 1: Branch setup + commit the spec & plan

**Files:**
- Commit (already in tree, uncommitted): `docs/superpowers/specs/2026-05-29-memory-phase5.md`, `docs/superpowers/plans/2026-05-29-memory-phase5.md`

- [ ] **Step 1: Confirm Phase 4 is merged into main**

Run:
```bash
git checkout main && git pull
git log --oneline -20 main | grep -iE 'phase 4|migration 0004|MaybeStartSweep|Sweeper'
```
Expected: the Phase 4 memory commits (`migration 0004`, `Sweeper`, `MaybeStartSweep`, etc.) appear on `main`. If they do NOT, **stop** — Phase 5 must not start until Phase 4 merges. (Do not branch from `ptolemy/memory-phase4`.)

- [ ] **Step 2: Cut the Phase 5 branch from clean main**

Run:
```bash
git checkout -b ptolemy/memory-phase5
```

- [ ] **Step 3: Commit the spec + plan**

```bash
git add docs/superpowers/specs/2026-05-29-memory-phase5.md docs/superpowers/plans/2026-05-29-memory-phase5.md
git commit -m "$(cat <<'EOF'
docs(memory): Phase 5 spec + implementation plan (supersession + dedup)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Verify pg_trgm is installable on the target DB** (one-time go/no-go; already confirmed available 1.6)

Run:
```bash
PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy \
  -c "SELECT name, default_version FROM pg_available_extensions WHERE name='pg_trgm';"
```
Expected: one row, `pg_trgm | 1.6`.

---

## Task 2: Migration 0005 — pg_trgm, indexes, status backfill

**Files:**
- Create: `internal/memory/migrations/0005_chunks_dedup_supersession.sql`
- Test: `internal/memory/migrations_test.go`

- [ ] **Step 1: Write the failing FS test**

Add to `internal/memory/migrations_test.go`:
```go
func TestMigrationsFS_Contains0005(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0005_chunks_dedup_supersession.sql")
	if err != nil {
		t.Fatalf("0005 migration missing from embedded FS: %v", err)
	}
	sql := string(data)
	for _, want := range []string{"pg_trgm", "chunks_content_trgm", "chunks_fact", "status = 'superseded'"} {
		if !strings.Contains(sql, want) {
			t.Errorf("0005 migration missing %q", want)
		}
	}
}
```
(`strings` is already imported in this file; if not, add it.)

- [ ] **Step 2: Run it red**

Run: `go test ./internal/memory/ -run TestMigrationsFS_Contains0005 -v`
Expected: FAIL — `0005 migration missing from embedded FS`.

- [ ] **Step 3: Create the migration**

`internal/memory/migrations/0005_chunks_dedup_supersession.sql`:
```sql
-- Phase 5: correction/redundancy layer.
-- pg_trgm was deferred from Phase 4 (NOT enabled there); enable it now.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Trigram GIN index for dedupRecent()'s similarity()/% candidate prefilter.
CREATE INDEX IF NOT EXISTS chunks_content_trgm
    ON chunks USING gin (content gin_trgm_ops);

-- Structured-fact ladder lookup (SPEC §5 step 1: same subject+predicate).
CREATE INDEX IF NOT EXISTS chunks_fact
    ON chunks (fact_subject, fact_predicate)
    WHERE fact_subject IS NOT NULL AND fact_predicate IS NOT NULL;

-- Unification backfill: bring legacy Phase-2-superseded rows (superseded_by set,
-- status still 'active') onto the unified status model so retrieval can rely on
-- status='active' alone. Idempotent; within the existing chunks_status_chk CHECK.
UPDATE chunks SET status = 'superseded'
 WHERE superseded_by IS NOT NULL AND status = 'active';
```

- [ ] **Step 4: Run the FS test green**

Run: `go test ./internal/memory/ -run TestMigrationsFS_Contains0005 -v`
Expected: PASS.

- [ ] **Step 5: Write the integration test**

Add to `internal/memory/migrations_test.go`:
```go
func TestApplyMigrations_0005DedupSupersession(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()

	var hasExt bool
	if err := conn.QueryRow(ctx,
		`SELECT exists(SELECT 1 FROM pg_extension WHERE extname='pg_trgm')`).Scan(&hasExt); err != nil {
		t.Fatal(err)
	}
	if !hasExt {
		t.Fatal("pg_trgm not installed after 0005")
	}
	for _, idx := range []string{"chunks_content_trgm", "chunks_fact"} {
		var ok bool
		if err := conn.QueryRow(ctx,
			`SELECT exists(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, idx).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("index %s missing after 0005", idx)
		}
	}

	// Backfill: a row with superseded_by set but status='active' must become 'superseded'.
	// Insert two rows, point one at the other, force status back to 'active', re-run the backfill.
	if _, err := conn.Exec(ctx,
		`INSERT INTO chunks (id, content, scope, status) VALUES ('new','n','global','active'),('old','o','global','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx,
		`UPDATE chunks SET superseded_by='new' WHERE id='old'`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx,
		`UPDATE chunks SET status='superseded' WHERE superseded_by IS NOT NULL AND status='active'`); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := conn.QueryRow(ctx, `SELECT status FROM chunks WHERE id='old'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "superseded" {
		t.Fatalf("backfill: old row status=%q, want 'superseded'", status)
	}
}
```
(Ensure `context` is imported in the test file.)

- [ ] **Step 6: Run the integration test green** (needs DB)

Run: `DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test ./internal/memory/ -run TestApplyMigrations_0005DedupSupersession -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/migrations/0005_chunks_dedup_supersession.sql internal/memory/migrations_test.go
git commit -m "$(cat <<'EOF'
feat(memory): migration 0005 — pg_trgm + dedup/fact indexes + status backfill

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Config — dedup knobs

**Files:**
- Modify: `internal/memory/config.go`
- Test: `internal/memory/config_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/memory/config_test.go`:
```go
func TestGCConfig_DedupDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://x")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("BRAIN_BASE_URL", "http://x")
	t.Setenv("BRAIN_MODEL", "m")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.GC.DedupEnabled {
		t.Errorf("DedupEnabled default = true, want false")
	}
	if cfg.GC.DedupThreshold != 0.7 {
		t.Errorf("DedupThreshold default = %v, want 0.7", cfg.GC.DedupThreshold)
	}
	if cfg.GC.DedupLookback != 24*time.Hour {
		t.Errorf("DedupLookback default = %v, want 24h", cfg.GC.DedupLookback)
	}
}

func TestGCConfig_RejectsBadDedup(t *testing.T) {
	base := func() {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("EMBEDDING_BASE_URL", "http://x")
		t.Setenv("EMBEDDING_MODEL", "m")
		t.Setenv("EMBEDDING_DIM", "768")
		t.Setenv("BRAIN_BASE_URL", "http://x")
		t.Setenv("BRAIN_MODEL", "m")
	}
	base()
	t.Setenv("GC_DEDUP_THRESHOLD", "1.5")
	if _, err := LoadConfig(); err == nil {
		t.Error("expected error for GC_DEDUP_THRESHOLD > 1")
	}
	t.Setenv("GC_DEDUP_THRESHOLD", "0.7")
	t.Setenv("GC_DEDUP_LOOKBACK", "30s")
	if _, err := LoadConfig(); err == nil {
		t.Error("expected error for GC_DEDUP_LOOKBACK < 1m")
	}
}
```

- [ ] **Step 2: Run them red**

Run: `go test ./internal/memory/ -run 'TestGCConfig_DedupDefaults|TestGCConfig_RejectsBadDedup' -v`
Expected: FAIL — `cfg.GC.DedupEnabled` etc. undefined.

- [ ] **Step 3: Add the fields + parsing + validation**

In `internal/memory/config.go`, extend `GCConfig`:
```go
type GCConfig struct {
	SweepEnabled     bool
	SweepInterval    time.Duration
	DecayLambda      float64
	ArchiveThreshold float64
	PurgeEnabled     bool
	PurgeGraceDays   float64

	// Phase 5 dedup knobs (placeholders — tune against real volume).
	DedupEnabled   bool          // GC_DEDUP_ENABLED (default false)
	DedupThreshold float64       // GC_DEDUP_THRESHOLD trigram candidate prefilter (default 0.7)
	DedupLookback  time.Duration // GC_DEDUP_LOOKBACK recent-row window (default 24h)
}
```
In `LoadConfig`, after the existing `cfg.GC = GCConfig{...}` block assigns the Phase 4 fields, add the three Phase 5 fields to the same struct literal:
```go
		DedupEnabled:   boolEnv("GC_DEDUP_ENABLED", false),
		DedupThreshold: floatEnv("GC_DEDUP_THRESHOLD", 0.7),
		DedupLookback:  durationEnv("GC_DEDUP_LOOKBACK", 24*time.Hour),
```
And after the existing GC validations add:
```go
	if cfg.GC.DedupThreshold <= 0 || cfg.GC.DedupThreshold > 1 {
		return MemoryConfig{}, fmt.Errorf("GC_DEDUP_THRESHOLD must be in (0,1], got %v", cfg.GC.DedupThreshold)
	}
	if cfg.GC.DedupLookback < time.Minute {
		return MemoryConfig{}, fmt.Errorf("GC_DEDUP_LOOKBACK must be >= 1m, got %v", cfg.GC.DedupLookback)
	}
```

- [ ] **Step 4: Run them green**

Run: `go test ./internal/memory/ -run 'TestGCConfig_DedupDefaults|TestGCConfig_RejectsBadDedup' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go
git commit -m "$(cat <<'EOF'
feat(memory): GC dedup config knobs (enabled/threshold/lookback)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Supersession store layer — one path

**Files:**
- Modify: `internal/memory/store.go`
- Test: `internal/memory/store_test.go`
- Modify (interface conformance): `internal/memory/orchestrator_test.go` (the fake `Store`)

This task introduces `markSupersededTx`, `Supersede`, `History`, `LookupFact`; reworks `SupersedeOnUpsert`; and **removes** `MarkSuperseded`.

- [ ] **Step 1: Write the failing tests** (replace the existing `TestPgStore_MarkSuperseded`)

In `internal/memory/store_test.go`, **delete** `TestPgStore_MarkSuperseded` and add:
```go
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
```
Add a small test helper at the bottom of `store_test.go` if `ptr` does not already exist:
```go
func ptr(s string) *string { return &s }
```

- [ ] **Step 2: Run them red**

Run: `DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test ./internal/memory/ -run 'TestPgStore_Supersede|TestPgStore_History' -v`
Expected: FAIL — `s.Supersede`, `s.History` undefined; `Chunk.Status/Supersedes/Version` not populated by `Get`.

- [ ] **Step 3: Update the `Store` interface + `Get` scan**

In `internal/memory/store.go`, change the interface:
```go
type Store interface {
	Upsert(ctx context.Context, chunks []Chunk) error
	Get(ctx context.Context, ids []string) ([]Chunk, error)

	// SupersedeOnUpsert atomically upserts the new chunks AND retires every chunk
	// whose id starts with "<oldDocID>#" via the unified version-chain model.
	SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error

	// Supersede inserts newChunks (active) and retires oldID, linking the chain:
	// old → status='superseded', superseded_by=newChunks[0].ID; newChunks[0] →
	// supersedes=oldID, version=old.version+1. Transactional + audited.
	Supersede(ctx context.Context, newChunks []Chunk, oldID string) error

	// History returns the full version chain for id, oldest → newest.
	History(ctx context.Context, id string) ([]Chunk, error)

	// LookupFact returns the most-recent active chunk for (factSubject, factPredicate),
	// or found=false when none exists. Drives the structured-fact ingest ladder.
	LookupFact(ctx context.Context, factSubject, factPredicate string) (chunk Chunk, found bool, err error)

	Reinforce(ctx context.Context, ids []string) error
	Stats(ctx context.Context) ([]ScopeStatusCount, error)
}
```
Remove the `MarkSuperseded` interface line and the `MarkSuperseded` method (lines ~102-105). Update `Get` to also scan the lifecycle columns needed by tests/History:
```go
func (s *PgStore) Get(ctx context.Context, ids []string) ([]Chunk, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT id, content, embedding, metadata, COALESCE(source,''), COALESCE(tenant_id,''),
		       published_at, valid_from, valid_to, superseded_by, created_at,
		       scope, status, version, supersedes
		FROM chunks WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		var emb pgvector.Vector
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.Content, &emb, &metaJSON, &c.Source, &c.Tenant,
			&c.PublishedAt, &c.ValidFrom, &c.ValidTo, &c.SupersededBy, &c.CreatedAt,
			&c.Scope, &c.Status, &c.Version, &c.Supersedes); err != nil {
			return nil, err
		}
		c.Embedding = emb.Slice()
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &c.Metadata)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Implement `markSupersededTx`, `Supersede`, `History`, `LookupFact`; rework `SupersedeOnUpsert`**

In `internal/memory/store.go`:
```go
// markSupersededTx retires each old id in favour of newID inside tx: status='superseded',
// superseded_by=newID, audited (active → superseded, 'supersession'). Matching zero rows is
// not an error. The caller must have inserted the new row(s) and set their supersedes/version.
func markSupersededTx(ctx context.Context, tx pgx.Tx, oldIDs []string, newID string) error {
	if len(oldIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		WITH moved AS (
		  UPDATE chunks SET status='superseded', superseded_by=$1
		   WHERE id = ANY($2) AND status='active'
		  RETURNING id
		)
		INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
		SELECT id, 'active', 'superseded', 'supersession' FROM moved
	`, newID, oldIDs); err != nil {
		return fmt.Errorf("mark superseded: %w", err)
	}
	return nil
}

func (s *PgStore) Supersede(ctx context.Context, newChunks []Chunk, oldID string) error {
	if len(newChunks) == 0 {
		return fmt.Errorf("Supersede: empty chunk slice")
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var oldVersion int
	if err := tx.QueryRow(ctx, `SELECT version FROM chunks WHERE id=$1`, oldID).Scan(&oldVersion); err != nil {
		return fmt.Errorf("supersede: load old %s: %w", oldID, err)
	}
	rep := newChunks[0].ID
	for i, c := range newChunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		var supersedes any
		version := 1
		if c.ID == rep {
			supersedes = oldID
			version = oldVersion + 1
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at,
			                    scope, confidence, fact_subject, fact_predicate, supersedes, version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (id) DO UPDATE SET
				content=EXCLUDED.content, embedding=EXCLUDED.embedding, metadata=EXCLUDED.metadata,
				source=EXCLUDED.source, tenant_id=EXCLUDED.tenant_id, published_at=EXCLUDED.published_at,
				scope=EXCLUDED.scope, confidence=EXCLUDED.confidence,
				fact_subject=EXCLUDED.fact_subject, fact_predicate=EXCLUDED.fact_predicate,
				supersedes=EXCLUDED.supersedes, version=EXCLUDED.version
		`, c.ID, c.Content, pgvector.NewVector(c.Embedding), meta, nullableStr(c.Source), nullableStr(c.Tenant),
			c.PublishedAt, scopeOrDefault(c.Scope), confidenceOrDefault(c.Confidence),
			c.FactSubject, c.FactPredicate, supersedes, version); err != nil {
			return fmt.Errorf("supersede insert %s: %w", c.ID, err)
		}
		_ = i
	}
	if err := markSupersededTx(ctx, tx, []string{oldID}, rep); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PgStore) History(ctx context.Context, id string) ([]Chunk, error) {
	rows, err := s.conn.Query(ctx, `
		WITH RECURSIVE history AS (
		  SELECT id, content, scope, status, version, supersedes, superseded_by FROM chunks WHERE id=$1
		  UNION ALL
		  SELECT c.id, c.content, c.scope, c.status, c.version, c.supersedes, c.superseded_by
		    FROM chunks c JOIN history h ON c.id = h.supersedes
		)
		SELECT id, content, scope, status, version, supersedes, superseded_by FROM history ORDER BY version
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Content, &c.Scope, &c.Status, &c.Version, &c.Supersedes, &c.SupersededBy); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PgStore) LookupFact(ctx context.Context, factSubject, factPredicate string) (Chunk, bool, error) {
	var c Chunk
	err := s.conn.QueryRow(ctx, `
		SELECT id, content, scope, status, version
		FROM chunks
		WHERE fact_subject=$1 AND fact_predicate=$2 AND status='active'
		ORDER BY version DESC, created_at DESC
		LIMIT 1
	`, factSubject, factPredicate).Scan(&c.ID, &c.Content, &c.Scope, &c.Status, &c.Version)
	if err == pgx.ErrNoRows {
		return Chunk{}, false, nil
	}
	if err != nil {
		return Chunk{}, false, fmt.Errorf("lookup fact: %w", err)
	}
	return c, true, nil
}
```
Rework `SupersedeOnUpsert` so its supersession half goes through `markSupersededTx` instead of the bare `UPDATE ... SET superseded_by`. Replace the existing post-upsert block:
```go
	// Representative new chunk; one pointer per supersession is enough.
	rep := chunks[0].ID
	var oldIDs []string
	rows, err := tx.Query(ctx, `SELECT id FROM chunks WHERE id LIKE $1 || '#%' AND status='active'`, supersedesOldDocID)
	if err != nil {
		return fmt.Errorf("supersede %s: %w", supersedesOldDocID, err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		oldIDs = append(oldIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := markSupersededTx(ctx, tx, oldIDs, rep); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
```
Add the helper near `scopeOrDefault`:
```go
func confidenceOrDefault(c string) string {
	if c == "" {
		return "normal"
	}
	return c
}
```

- [ ] **Step 5: Update the fake `Store` in `orchestrator_test.go`**

The fake type is `fakeStore` (top of `internal/memory/orchestrator_test.go`) with existing fields `upserted []Chunk`, `supersedeCalls []supersedeCall` (used by `SupersedeOnUpsert`), `reinforced [][]string`, `statsCalls int`. Add two new fields:
```go
	rowSupersedeCalls []string // records each Supersede(newChunks, oldID) call's oldID
	factHit           *Chunk   // when non-nil, LookupFact returns it as found
```
**Delete** the existing `MarkSuperseded` method (lines ~27-29). Add the three new interface methods (recording calls in the same style as the existing fakes):
```go
func (f *fakeStore) Supersede(_ context.Context, newChunks []Chunk, oldID string) error {
	f.upserted = append(f.upserted, newChunks...)
	f.rowSupersedeCalls = append(f.rowSupersedeCalls, oldID)
	return nil
}
func (f *fakeStore) History(_ context.Context, _ string) ([]Chunk, error) { return nil, nil }
func (f *fakeStore) LookupFact(_ context.Context, _, _ string) (Chunk, bool, error) {
	if f.factHit != nil {
		return *f.factHit, true, nil
	}
	return Chunk{}, false, nil
}
```

- [ ] **Step 6: Run all the store/orchestrator tests green** (needs DB for the store integration tests; unit ones run without)

Run: `DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test ./internal/memory/ -run 'TestPgStore_Supersede|TestPgStore_History|TestOrchestrator' -v`
Expected: PASS. Also confirm the package compiles: `go build ./...`.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/store.go internal/memory/store_test.go internal/memory/orchestrator_test.go
git commit -m "$(cat <<'EOF'
feat(memory): unify supersession via markSupersededTx + Supersede/History/LookupFact

Single version-chain path: old→superseded+superseded_by, new→supersedes+version+1,
audited. SupersedeOnUpsert reworked onto the helper; MarkSuperseded removed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Retrieval — single `status='active'` filter

**Files:**
- Modify: `internal/memory/bm25_retriever.go:45`, `internal/memory/hybrid_retriever.go` (lines ~40, 49, 63), `internal/memory/retriever.go:40`
- Test: `internal/memory/hybrid_retriever_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/memory/hybrid_retriever_test.go` (modeled on the existing `TestHybridRetriever_ExcludesNonActive`, which uses `freshDB` + `fakeEmbedder{vecs:...}`):
```go
func TestHybridRetriever_ExcludesSupersededByStatusAlone(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	// Two rows: one active, one superseded. Deliberately leave superseded_by NULL on the
	// superseded row so the test proves status='active' (NOT superseded_by IS NULL) excludes it.
	_ = s.Upsert(ctx, []Chunk{
		{ID: "live", Content: "alpha bravo keyword", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "global"},
		{ID: "stale", Content: "alpha bravo keyword", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "global"},
	})
	if _, err := conn.Exec(ctx, `UPDATE chunks SET status='superseded' WHERE id='stale'`); err != nil {
		t.Fatal(err)
	}
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(ctx, Query{Text: "keyword", K: 10}, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.ID == "stale" {
			t.Fatalf("retrieval returned superseded row 'stale'")
		}
	}
}
```

- [ ] **Step 2: Run it red**

Run: `DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test ./internal/memory/ -run TestHybridRetriever_ExcludesSupersededByStatusAlone -v`
Expected: PASS already? No — it passes today because the current query also has `superseded_by IS NULL`, but `stale` has `superseded_by=NULL`, so the only thing excluding it is `status='active'` — which is already present. **This test should pass before the edit** and is a guard that the *upcoming* removal of `superseded_by IS NULL` does not regress. If it fails, the status filter is missing somewhere — fix that first.

> Note: this task's "red" is structural, not behavioral — the behavioral guarantee already holds via `status='active'`. The edit below *removes the now-redundant clause*; the test ensures status alone still excludes superseded rows.

- [ ] **Step 3: Remove `superseded_by IS NULL` from all arms**

`bm25_retriever.go` — delete the line `AND superseded_by IS NULL` (keep `AND status = 'active'`).

`hybrid_retriever.go` — in `hybridRrfQuery`, delete all three `superseded_by IS NULL` clauses:
- the `bm25` CTE `WHERE` (keep `status='active'` + `published_at`),
- the `vec` CTE `WHERE`,
- the outer `WHERE c.superseded_by IS NULL` (keep `c.status='active'` + `c.published_at`).

`retriever.go` (`VectorRetriever`) — change `WHERE superseded_by IS NULL` to `WHERE status = 'active'` (closes the Phase 4 gap where the vector arm leaked archived/dead rows).

- [ ] **Step 4: Run the test + eval guard green**

Run: `DATABASE_URL='...' go test ./internal/memory/ -run 'TestHybridRetriever|TestBm25|TestVector' -v`
Expected: PASS.

- [ ] **Step 5: No-regression eval gate**

Run (full live ingest + eval): `make eval-memory`
Expected: `mean recall@5 = 1.000` over 30 questions. If not 1.000, the filter change regressed retrieval — stop and investigate before committing.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/bm25_retriever.go internal/memory/hybrid_retriever.go internal/memory/retriever.go internal/memory/hybrid_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): retrieval filters status='active' alone (drop superseded_by IS NULL)

Single supersession read filter across bm25/hybrid/vector arms; closes the
VectorRetriever status-filter gap. Recall@5 still 1.000.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Structured-fact ladder + confidence at ingest

**Files:**
- Modify: `internal/memory/orchestrator.go:27-70` (`Ingest`)
- Test: `internal/memory/orchestrator_test.go`
- Add helper: `internal/memory/store.go` (`normalizeContent`)

- [ ] **Step 1: Write the failing unit tests**

Add to `internal/memory/orchestrator_test.go` (using the inline `&Orchestrator{...}` construction the existing ingest tests use — `FixedSizeChunker` + `fakeEmbedder`; short single-chunk docs need exactly one vec):
```go
func newFactOrch(fs *fakeStore) *Orchestrator {
	return &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1}}}, // one chunk → one vec
		Store:    fs,
	}
}

func TestOrchestrator_Ingest_NoFact_Upserts(t *testing.T) {
	fs := &fakeStore{}
	if err := newFactOrch(fs).Ingest(context.Background(), RawDocument{ID: "d1", Text: "hello world"}); err != nil {
		t.Fatal(err)
	}
	if len(fs.upserted) == 0 {
		t.Error("expected Upsert for non-fact doc")
	}
	if len(fs.rowSupersedeCalls) != 0 || len(fs.reinforced) != 0 {
		t.Error("non-fact doc must not Supersede or Reinforce")
	}
}

func TestOrchestrator_Ingest_FactDuplicate_Reinforces(t *testing.T) {
	fs := &fakeStore{factHit: &Chunk{ID: "old", Content: "deploy target is AWS"}}
	err := newFactOrch(fs).Ingest(context.Background(), RawDocument{
		ID:       "d2",
		Text:     "deploy target is AWS",
		Metadata: map[string]any{"fact_subject": "deploy", "fact_predicate": "target"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.reinforced) != 1 || len(fs.reinforced[0]) != 1 || fs.reinforced[0][0] != "old" {
		t.Errorf("expected Reinforce([old]) for duplicate fact, got %v", fs.reinforced)
	}
	if len(fs.upserted) != 0 || len(fs.rowSupersedeCalls) != 0 {
		t.Error("duplicate fact must not Upsert or Supersede")
	}
}

func TestOrchestrator_Ingest_FactContradiction_Supersedes(t *testing.T) {
	fs := &fakeStore{factHit: &Chunk{ID: "old", Content: "deploy target is AWS"}}
	err := newFactOrch(fs).Ingest(context.Background(), RawDocument{
		ID:       "d3",
		Text:     "deploy target is GCP",
		Metadata: map[string]any{"fact_subject": "deploy", "fact_predicate": "target"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.rowSupersedeCalls) != 1 || fs.rowSupersedeCalls[0] != "old" {
		t.Errorf("expected Supersede(old), got %v", fs.rowSupersedeCalls)
	}
}
```

- [ ] **Step 2: Run them red**

Run: `go test ./internal/memory/ -run 'TestOrchestrator_Ingest_Fact|TestOrchestrator_Ingest_NoFact' -v`
Expected: FAIL — ladder not implemented (`Supersede`/`Reinforce` not called on the fact branches).

- [ ] **Step 3: Add `normalizeContent` to `store.go`**

```go
// normalizeContent trims and collapses internal whitespace, case-sensitive
// (content can be code/identifiers where case is meaningful). Used by the
// structured-fact ladder and dedup to decide content equality.
func normalizeContent(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
```
Add `"strings"` to `store.go` imports.

- [ ] **Step 4: Implement the ladder in `Ingest`**

In `internal/memory/orchestrator.go`, replace the scope/supersedes/upsert tail (lines ~59-69) with:
```go
	scope := "global"
	if raw, ok := doc.Metadata["scope"].(string); ok && raw != "" {
		scope = raw
	}
	confidence := "normal"
	if raw, ok := doc.Metadata["confidence"].(string); ok && raw != "" {
		confidence = raw
	}
	factSubject, _ := doc.Metadata["fact_subject"].(string)
	factPredicate, _ := doc.Metadata["fact_predicate"].(string)
	for i := range chunks {
		chunks[i].Scope = scope
		chunks[i].Confidence = confidence
		if factSubject != "" && factPredicate != "" {
			fs, fp := factSubject, factPredicate
			chunks[i].FactSubject = &fs
			chunks[i].FactPredicate = &fp
		}
	}

	if old, ok := doc.Metadata["supersedes"].(string); ok && old != "" {
		return o.Store.SupersedeOnUpsert(ctx, chunks, old)
	}

	// Structured-fact ladder (SPEC §5 step 1, ~0ms, one indexed lookup).
	if factSubject != "" && factPredicate != "" {
		existing, found, err := o.Store.LookupFact(ctx, factSubject, factPredicate)
		if err != nil {
			return fmt.Errorf("lookup fact: %w", err)
		}
		if found {
			if normalizeContent(existing.Content) == normalizeContent(chunks[0].Content) {
				return o.Store.Reinforce(ctx, []string{existing.ID}) // duplicate
			}
			return o.Store.Supersede(ctx, chunks, existing.ID) // correction
		}
	}
	return o.Store.Upsert(ctx, chunks)
```

- [ ] **Step 5: Run them green**

Run: `go test ./internal/memory/ -run 'TestOrchestrator_Ingest' -v`
Expected: PASS.

- [ ] **Step 6: Integration test — confidence news flow** (needs DB)

Add to `internal/memory/store_test.go` (or `orchestrator_test.go` with a real store):
```go
func TestOrchestrator_Confidence_NewsFlow(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	// low-confidence first report
	_ = s.Upsert(ctx, []Chunk{{ID: "n1", Content: "quake magnitude 5.0", Embedding: []float32{1, 0, 0, 0},
		PublishedAt: now, Scope: "global", Confidence: "low", FactSubject: ptr("quake"), FactPredicate: ptr("magnitude")}})
	// high-confidence verified correction supersedes it
	_ = s.Supersede(ctx, []Chunk{{ID: "n2", Content: "quake magnitude 5.4", Embedding: []float32{0, 1, 0, 0},
		PublishedAt: now, Scope: "global", Confidence: "high", FactSubject: ptr("quake"), FactPredicate: ptr("magnitude")}}, "n1")

	hist, err := s.History(ctx, "n2")
	if err != nil || len(hist) != 2 {
		t.Fatalf("History len=%d err=%v, want 2", len(hist), err)
	}
	// original kept + linked, only the high-confidence row is active
	got, _ := s.Get(ctx, []string{"n1", "n2"})
	for _, c := range got {
		if c.ID == "n1" && c.Status != "superseded" {
			t.Errorf("low report n1 status=%q, want superseded", c.Status)
		}
		if c.ID == "n2" && c.Status != "active" {
			t.Errorf("high correction n2 status=%q, want active", c.Status)
		}
	}
}
```

- [ ] **Step 7: Run it green** (needs DB)

Run: `DATABASE_URL='...' go test ./internal/memory/ -run TestOrchestrator_Confidence_NewsFlow -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/orchestrator_test.go internal/memory/store.go internal/memory/store_test.go
git commit -m "$(cat <<'EOF'
feat(memory): structured-fact ingest ladder + confidence (dup→reinforce, correction→supersede)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Dedup pass — `dedupRecent()`, gated

**Files:**
- Modify: `internal/memory/sweep.go`
- Test: `internal/memory/sweep_test.go`

Collapse rule: among trigram candidates (same scope, `similarity >= threshold`), collapse **iff** `normalizeContent` is equal. That makes a contradiction (different content) unconditionally safe.

- [ ] **Step 1: Write the failing tests**

Add to `internal/memory/sweep_test.go`:
```go
func dedupCfg(enabled bool) GCConfig {
	return GCConfig{
		DedupEnabled:   enabled,
		DedupThreshold: 0.5,
		DedupLookback:  24 * time.Hour,
		DecayLambda:    0.05,
		ArchiveThreshold: 0.1,
	}
}

func TestSweeper_Dedup_ContradictionPairBothSurvive(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	// High trigram similarity but NOT normalized-equal: a contradiction.
	_ = s.Upsert(ctx, []Chunk{
		{ID: "c8088", Content: "the service runs on port 8088", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "project"},
		{ID: "c1089", Content: "the service runs on port 1089", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "project"},
	})
	sw := NewSweeper(conn, dedupCfg(true))
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, []string{"c8088", "c1089"})
	for _, c := range got {
		if c.Status != "active" {
			t.Errorf("contradiction row %s status=%q, want active (never collapsed)", c.ID, c.Status)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected both contradiction rows to survive, got %d", len(got))
	}
}

func TestSweeper_Dedup_NearIdenticalCollapses(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	old := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	// Normalized-equal (differ only by whitespace). Older = survivor.
	_, _ = conn.Exec(ctx, `INSERT INTO chunks (id, content, scope, status, created_at, access_count)
	  VALUES ('keep','user prefers tabs','project','active',$1,3),
	         ('drop','user   prefers   tabs','project','active',$2,0)`, old, newer)
	sw := NewSweeper(conn, dedupCfg(true))
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	var keepStatus, dropStatus string
	_ = conn.QueryRow(ctx, `SELECT status FROM chunks WHERE id='keep'`).Scan(&keepStatus)
	_ = conn.QueryRow(ctx, `SELECT status FROM chunks WHERE id='drop'`).Scan(&dropStatus)
	if keepStatus != "active" {
		t.Errorf("survivor keep status=%q, want active", keepStatus)
	}
	if dropStatus != "dead" {
		t.Errorf("loser drop status=%q, want dead", dropStatus)
	}
	var ac int
	_ = conn.QueryRow(ctx, `SELECT access_count FROM chunks WHERE id='keep'`).Scan(&ac)
	if ac < 4 {
		t.Errorf("survivor access_count=%d, want reinforced (>3)", ac)
	}
	var auditN int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM chunk_audit WHERE chunk_id='drop' AND new_status='dead' AND reason='duplicate'`).Scan(&auditN)
	if auditN != 1 {
		t.Errorf("expected 1 dead/duplicate audit row for drop, got %d", auditN)
	}
}

func TestSweeper_Dedup_GatedOffIsNoOp(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, _ = conn.Exec(ctx, `INSERT INTO chunks (id, content, scope, status, created_at)
	  VALUES ('a','same text','project','active',$1),('b','same text','project','active',$1)`, now)
	sw := NewSweeper(conn, dedupCfg(false)) // gated OFF
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE status='active'`).Scan(&n)
	if n != 2 {
		t.Errorf("gated-off dedup changed rows: active=%d, want 2", n)
	}
}
```

- [ ] **Step 2: Run them red**

Run: `DATABASE_URL='...' go test ./internal/memory/ -run 'TestSweeper_Dedup' -v`
Expected: FAIL — `sw.dedupRecent` undefined.

- [ ] **Step 3: Implement `dedupRecent`**

Add to `internal/memory/sweep.go`:
```go
// dedupRecent collapses normalized-content-identical near-duplicates among active rows
// created within DedupLookback, within scope. Trigram similarity (>= DedupThreshold) only
// PREFILTERS candidate pairs; the collapse decision is normalized-content EQUALITY, so a
// contradiction (different content) is never collapsed. No-op unless DedupEnabled.
func (s *Sweeper) dedupRecent(ctx context.Context) error {
	if !s.cfg.DedupEnabled {
		return nil
	}
	// Recent active rows, newest first so we keep the established (older) row as survivor.
	rows, err := s.conn.Query(ctx, `
		SELECT id, content, scope, access_count, created_at
		FROM chunks
		WHERE status='active' AND created_at >= now() - make_interval(secs => $1::float8)
		ORDER BY created_at DESC
	`, s.cfg.DedupLookback.Seconds())
	if err != nil {
		return fmt.Errorf("dedup recent scan: %w", err)
	}
	type row struct {
		id, content, scope string
		accessCount        int
		createdAt          time.Time
	}
	var recent []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.content, &r.scope, &r.accessCount, &r.createdAt); err != nil {
			rows.Close()
			return err
		}
		recent = append(recent, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	dead := map[string]bool{}
	for _, r := range recent {
		if dead[r.id] {
			continue
		}
		// Trigram candidates in the same scope (set similarity threshold per-statement).
		crows, err := s.conn.Query(ctx, `
			SELECT id, content, access_count, created_at
			FROM chunks
			WHERE status='active' AND scope=$1 AND id <> $2
			  AND similarity(content, $3) >= $4::float4
		`, r.scope, r.id, r.content, s.cfg.DedupThreshold)
		if err != nil {
			return fmt.Errorf("dedup candidates: %w", err)
		}
		type cand struct {
			id, content string
			accessCount int
			createdAt   time.Time
		}
		var cands []cand
		for crows.Next() {
			var c cand
			if err := crows.Scan(&c.id, &c.content, &c.accessCount, &c.createdAt); err != nil {
				crows.Close()
				return err
			}
			cands = append(cands, c)
		}
		crows.Close()
		if err := crows.Err(); err != nil {
			return err
		}

		for _, c := range cands {
			if dead[c.id] {
				continue
			}
			if normalizeContent(r.content) != normalizeContent(c.content) {
				continue // similar but not identical → keep both (safe fallback / contradiction)
			}
			// Survivor: higher access_count; tie-break older created_at.
			survID, survAC := r.id, r.accessCount
			loseID := c.id
			if c.accessCount > r.accessCount || (c.accessCount == r.accessCount && c.createdAt.Before(r.createdAt)) {
				survID, survAC = c.id, c.accessCount
				loseID = r.id
			}
			if err := s.collapseDuplicate(ctx, survID, loseID); err != nil {
				return err
			}
			dead[loseID] = true
			_ = survAC
			if loseID == r.id {
				break // r itself is gone; stop pairing it
			}
		}
	}
	return nil
}

// collapseDuplicate reinforces the survivor and marks the loser dead 'duplicate', audited,
// in one transaction.
func (s *Sweeper) collapseDuplicate(ctx context.Context, survivorID, loserID string) error {
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin dedup tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`UPDATE chunks SET access_count=access_count+1, last_accessed_at=now() WHERE id=$1`, survivorID); err != nil {
		return fmt.Errorf("reinforce survivor: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE chunks SET status='dead', dead_at=now() WHERE id=$1 AND status='active'`, loserID); err != nil {
		return fmt.Errorf("kill loser: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason) VALUES ($1,'active','dead','duplicate')`, loserID); err != nil {
		return fmt.Errorf("dedup audit: %w", err)
	}
	return tx.Commit(ctx)
}
```
Ensure `sweep.go` imports `time` (already) and `github.com/jackc/pgx/v5` (already).

- [ ] **Step 4: Run them green** (needs DB)

Run: `DATABASE_URL='...' go test ./internal/memory/ -run 'TestSweeper_Dedup' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/sweep.go internal/memory/sweep_test.go
git commit -m "$(cat <<'EOF'
feat(memory): gated dedupRecent — collapse on content equality, contradictions kept

Trigram only prefilters candidates; collapse iff normalizeContent equal, so a
contradiction is never collapsed. Loser → dead/'duplicate' (audited); gated by
GC_DEDUP_ENABLED (default off).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Sweep pass order + shutdown-race fix

**Files:**
- Modify: `internal/memory/sweep.go` (`sweepOnce`, `Run`)
- Modify: `internal/memory/module.go` (`MaybeStartSweep`)
- Modify: `cmd/workerd/main.go`
- Test: `internal/memory/module_test.go`, `internal/memory/sweep_test.go`

- [ ] **Step 1: Write the failing test for the done-channel stop**

Add to `internal/memory/module_test.go` (or `sweep_test.go` — it needs no DB):
```go
func TestSweeper_Run_SignalsDoneOnCancel(t *testing.T) {
	sw := NewSweeper(nil, GCConfig{SweepInterval: time.Hour}) // ticker never fires within the test
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go sw.Run(ctx, done)
	cancel()
	select {
	case <-done:
		// ok: Run returned and closed done
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not signal done within 2s of cancel")
	}
}
```

- [ ] **Step 2: Run it red**

Run: `go test ./internal/memory/ -run TestSweeper_Run_SignalsDoneOnCancel -v`
Expected: FAIL — `Run` takes one arg (signature mismatch / won't compile).

- [ ] **Step 3: Update `sweepOnce` order + `Run` signature**

In `internal/memory/sweep.go`:
```go
func (s *Sweeper) sweepOnce(ctx context.Context) error {
	if err := s.dedupRecent(ctx); err != nil {
		return fmt.Errorf("dedup pass: %w", err)
	}
	if err := s.archiveDecayed(ctx); err != nil {
		return fmt.Errorf("archive pass: %w", err)
	}
	if err := s.purgeDead(ctx); err != nil {
		return fmt.Errorf("purge pass: %w", err)
	}
	return nil
}

// Run ticks every cfg.SweepInterval and runs a sweep. Closes done when it returns
// (on ctx cancellation) so the caller can wait before tearing down the connection.
func (s *Sweeper) Run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	log.Info().Dur("interval", s.cfg.SweepInterval).Msg("memory sweep loop started")
	runLoop(ctx, s.cfg.SweepInterval, s.sweepOnce)
	log.Info().Msg("memory sweep loop stopped")
}
```

- [ ] **Step 4: Update `MaybeStartSweep` to wait on done**

In `internal/memory/module.go`, replace the goroutine launch + cleanup:
```go
	sw := NewSweeper(conn, cfg.GC)
	sweepCtx, cancelSweep := context.WithCancel(ctx)
	done := make(chan struct{})
	go sw.Run(sweepCtx, done)
	cleanup = func() {
		cancelSweep()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Warn().Msg("memory sweep did not stop within 5s; closing connection anyway")
		}
		_ = conn.Close(context.Background())
	}
	return cleanup, true, nil
```
Add imports to `module.go`: `"time"` and `"github.com/rs/zerolog/log"`.

- [ ] **Step 5: Update workerd shutdown**

In `cmd/workerd/main.go`, the existing call site already gets `cleanup`/`stop` from `MaybeStartSweep` and calls it on shutdown. Confirm shutdown calls `cleanup()` (now the waiting version) **before** the process exits; no signature change is needed (the func is still `func()`). If the call site manually cancels a separate context, remove that — cancellation now lives inside `cleanup`.

- [ ] **Step 6: Run the test green + existing sweep tests** (the done-channel test needs no DB)

Run: `go test ./internal/memory/ -run TestSweeper_Run_SignalsDoneOnCancel -v && go build ./...`
Expected: PASS + clean build. Also run any existing `TestSweeper_*` (DB) to confirm `sweepOnce` ordering didn't break them: `DATABASE_URL='...' go test ./internal/memory/ -run TestSweeper -v`.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/sweep.go internal/memory/module.go internal/memory/module_test.go cmd/workerd/main.go
git commit -m "$(cat <<'EOF'
feat(memory): sweep order dedup→archive→purge; stop waits for Run (shutdown-race fix)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Eval fixtures + dedup measurement mode

**Files:**
- Create: `internal/memory/eval/testdata/gc/contradiction.md`, `.../duplicate.md`, `.../news.md` (separate from the recall corpus)
- Modify: `internal/memory/eval/eval.go` (add `MeasureDedup`)
- Modify: `cmd/memory-eval/main.go` (add `-dedup` flag)
- Modify: `Makefile` (add `eval-memory-dedup`)

This mode dry-runs the dedup *predicate* (normalized-content equality among trigram candidates) over the frozen recall corpus and reports would-collapse count + recall before/after — the artifact for acceptance #5. It does NOT mutate rows.

- [ ] **Step 1: Write the failing test for the measurement function**

Add to `internal/memory/eval/eval_test.go`:
```go
func TestMeasureDedup_CountsNormalizedEqualPairs(t *testing.T) {
	docs := []memory.RawDocument{
		{ID: "a", Text: "user prefers tabs"},
		{ID: "b", Text: "user   prefers tabs"}, // normalized-equal to a
		{ID: "c", Text: "deploy target is AWS"},
		{ID: "d", Text: "deploy target is GCP"}, // similar but NOT equal → not a collapse
	}
	n := memory.MeasureDedupCollapses(docs)
	if n != 1 {
		t.Fatalf("would-collapse count = %d, want 1", n)
	}
}
```
> `MeasureDedupCollapses` is a pure, DB-free function on `RawDocument.Text` so the measurement is unit-testable. Place it in `internal/memory` (exported) and call it from the eval harness. It mirrors `dedupRecent`'s predicate: count pairs that are `normalizeContent`-equal (the in-DB version additionally trigram-prefilters, but equality is the collapse bar, so the count matches).

- [ ] **Step 2: Run it red**

Run: `go test ./internal/memory/eval/ -run TestMeasureDedup -v`
Expected: FAIL — `MeasureDedupCollapses` undefined.

- [ ] **Step 3: Implement `MeasureDedupCollapses` in `internal/memory`**

Add to `internal/memory/sweep.go` (or a new `internal/memory/dedup_measure.go`):
```go
// MeasureDedupCollapses returns how many documents would be collapsed as duplicates
// (normalized-content equal to an earlier doc). Pure + DB-free, for the eval harness's
// dedup-redundancy report. Mirrors dedupRecent's collapse bar (content equality).
func MeasureDedupCollapses(docs []RawDocument) int {
	seen := map[string]bool{}
	collapses := 0
	for _, d := range docs {
		key := normalizeContent(d.Text)
		if seen[key] {
			collapses++
			continue
		}
		seen[key] = true
	}
	return collapses
}
```

- [ ] **Step 4: Run it green**

Run: `go test ./internal/memory/eval/ -run TestMeasureDedup -v`
Expected: PASS.

- [ ] **Step 5: Add the GC fixtures (separate from recall corpus)**

`internal/memory/eval/testdata/gc/contradiction.md`:
```markdown
---
published: 2026-05-29T00:00:00Z
fact_subject: deploy
fact_predicate: target
---
The deployment target is AWS.
```
`internal/memory/eval/testdata/gc/duplicate.md`:
```markdown
---
published: 2026-05-29T00:00:00Z
---
The user prefers gofmt with the %w error-wrapping convention.
```
`internal/memory/eval/testdata/gc/news.md`:
```markdown
---
published: 2026-05-29T00:00:00Z
confidence: low
fact_subject: quake
fact_predicate: magnitude
---
Initial reports put the earthquake magnitude at 5.0.
```
> These are reference fixtures for the integration tests in Tasks 4/6/7 and for manual exploration; they are intentionally NOT loaded by `eval-memory` (recall corpus), only by `eval-memory-dedup` if you wire them, or read directly by tests. Keep them human-authored and frozen.

- [ ] **Step 6: Add `MeasureDedup` to the eval harness + `-dedup` flag**

In `internal/memory/eval/eval.go`, add a function that loads the frozen recall corpus and reports the redundancy:
```go
// MeasureDedup loads the fixture corpus and reports how many docs the dedup pass
// would collapse (normalized-content equality), plus the corpus size. DB-free.
func MeasureDedup(fixtureDir string) (corpusSize, wouldCollapse int, err error) {
	docs, err := LoadFixtureCorpus(fixtureDir)
	if err != nil {
		return 0, 0, err
	}
	return len(docs), memory.MeasureDedupCollapses(docs), nil
}
```
In `cmd/memory-eval/main.go`, add a `-dedup` bool flag (mirror the existing `-sweep` flag wiring). When set:
```go
if *dedupFlag {
	size, collapses, err := eval.MeasureDedup(os.Getenv("RAG_FIXTURE_DIR"))
	if err != nil {
		log.Fatalf("measure dedup: %v", err)
	}
	fmt.Printf("dedup-redundancy: corpus=%d would_collapse=%d (verdict: %s)\n",
		size, collapses, map[bool]string{true: "SAFE to leave gated; collapses>0 means review", false: "no redundancy in corpus"}[collapses > 0])
	return
}
```
(Match the file's existing flag/import style — `flag`, `fmt`, `os`, `log`, and the `eval` package path.)

- [ ] **Step 7: Add the Makefile target**

In `Makefile`, after `eval-memory-sweep`:
```makefile
eval-memory-dedup: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED) -dedup
```

- [ ] **Step 8: Run the measurement + verify build**

Run: `go build ./... && make eval-memory-dedup`
Expected: prints `dedup-redundancy: corpus=12 would_collapse=0 ...` (the curated recall fixtures are distinct → 0 collapses → safe to keep dedup gated). If `would_collapse > 0`, that is the signal to keep the gate off and investigate.

- [ ] **Step 9: Commit**

```bash
git add internal/memory/sweep.go internal/memory/eval/eval.go internal/memory/eval/eval_test.go cmd/memory-eval/main.go Makefile internal/memory/eval/testdata/gc/
git commit -m "$(cat <<'EOF'
feat(memory/eval): dedup-redundancy measurement mode + GC fixtures

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Docs touch-ups + full verification

**Files:**
- Modify: `docs/memory/DATA_MODEL.md` (fix stale `version TEXT` → `INTEGER`; note 0005 indexes/extension)
- Modify: `docs/memory/TASKS-GC.md` (tick Phases 4–5 supersession/structured/dedup items)
- Modify: `docs/memory/IMPLEMENTATION_PLAN.md` (add a Phase 5 section with file/test pointers)
- Modify: `.env.example` (document the 3 dedup env vars)

- [ ] **Step 1: Fix DATA_MODEL.md**

In `docs/memory/DATA_MODEL.md`, change the line `ADD COLUMN version TEXT` note to `INTEGER NOT NULL DEFAULT 1`, and add a short "Phase 5 (0005)" note listing `pg_trgm`, `chunks_content_trgm`, `chunks_fact`, and the status backfill.

- [ ] **Step 2: Tick TASKS-GC.md**

In `docs/memory/TASKS-GC.md`, mark the Phase 4 (Supersession) and Phase 5 (Dedup) checkboxes `[x]` that this PR implements: `Supersede()`, `History()`, structured-fact path, `confidence`, `dedupRecent()`, sweep pass order.

- [ ] **Step 3: Add a Phase 5 section to IMPLEMENTATION_PLAN.md**

Mirror the existing Phase 4 section format: file pointers + test names from this plan.

- [ ] **Step 4: Document the env vars**

In `.env.example`, add (with the placeholder-tuning note matching the existing GC block):
```bash
# Phase 5 dedup (placeholders — tune against real volume; gated off by default)
GC_DEDUP_ENABLED=false
GC_DEDUP_THRESHOLD=0.7
GC_DEDUP_LOOKBACK=24h
```

- [ ] **Step 5: Full verification**

Run, in order:
```bash
go build ./...
make test
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test ./internal/memory/... -v
make eval-memory
make eval-memory-dedup
```
Expected: build clean; `make test` green; integration tests green; `make eval-memory` = **recall@5 1.000**; `eval-memory-dedup` prints corpus + would-collapse. Confirm coverage held: `go test ./internal/... -cover` ≥ 80%.

- [ ] **Step 6: Commit**

```bash
git add docs/memory/DATA_MODEL.md docs/memory/TASKS-GC.md docs/memory/IMPLEMENTATION_PLAN.md .env.example
git commit -m "$(cat <<'EOF'
docs(memory): Phase 5 — DATA_MODEL/TASKS/IMPLEMENTATION + dedup env vars

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist (run before requesting code review)

- [ ] **Cardinal rule:** `TestSweeper_Dedup_ContradictionPairBothSurvive` passes — a contradiction is never collapsed. Collapse bar is content *equality*, not similarity.
- [ ] **One supersession path:** `MarkSuperseded` removed; `SupersedeOnUpsert` + `Supersede` both go through `markSupersededTx`; retrieval filters `status='active'` alone in all four arms (bm25, hybrid×3, VectorRetriever).
- [ ] **Recall neutral:** `make eval-memory` = 1.000 after migration 0005.
- [ ] **Measured before enabled:** `eval-memory-dedup` reports redundancy; `GC_DEDUP_ENABLED` ships `false`.
- [ ] **Config, not magic:** `GC_DEDUP_THRESHOLD` + `GC_DEDUP_LOOKBACK` are env-driven and validated.
- [ ] **Shutdown race fixed:** `Run` closes `done`; `MaybeStartSweep` cleanup waits on it before closing the conn.
- [ ] **No placeholders / undefined refs:** `go build ./...` and `make test` both clean.

## Execution handoff

After the plan is approved, dispatch via **superpowers:subagent-driven-development** — one fresh subagent per task, two-stage review between tasks. Tasks 2–9 each end at a green test + commit; Task 1 (branch) gates on the Phase 4 merge.
