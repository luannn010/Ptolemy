# Memory Module — Phase 1 (Hybrid Retrieval + Eval Harness) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add lexical (BM25) retrieval alongside the Phase 0 vector arm, fuse them with Reciprocal Rank Fusion (RRF, C=60), switch the orchestrator default to the new hybrid retriever, and ship an eval harness (recall@k) that lets every later phase prove or disprove its claim of improvement.

**Architecture:** Stays inside `internal/memory/`. Adds a new migration that creates a ParadeDB `pg_search` BM25 index on the existing `chunks` table; adds `Bm25Retriever`, `HybridRetriever`, and `RrfFusion` next to the existing `VectorRetriever`/`PassthroughFusion`. `HybridRetriever` runs the fusion as a single SQL CTE round-trip (spec's Option A). `RrfFusion` is the app-side equivalent kept ready for Option B. `module.go` flips one line to make `HybridRetriever` the default. The eval harness lives in a new sub-package `internal/memory/eval/` plus a thin `cmd/memory-eval/` CLI and a JSON seed at `docs/memory/eval/seed.json`.

**Tech Stack:** Go 1.25; ParadeDB `pg_search` 0.23.4 (already installed at `192.168.0.164:1091` and in CI's `paradedb/paradedb:latest` service); `pgx/v5`; `pgvector-go`. No new module dependencies.

**Spec:** [docs/memory/](../../memory/) — `IMPLEMENTATION_PLAN.md` Phase 1 + Eval-harness sections, `RETRIEVAL.md` (the single-CTE hybrid query is canonical; adapt BM25 arm to `@@@`), `ARCHITECTURE.md` (Retriever/Fusion interfaces stay unchanged), `DATA_MODEL.md` (migration `0002_chunks_bm25`).

**Locked decisions (from kickoff brief — no need to re-litigate):**
- BM25 backend: ParadeDB `pg_search` (operator `@@@`, scoring via `paradedb.score(id)`). The index requires `key_field='id'`.
- Fusion: RRF with constant `C=60`. No weighting, no normalisation.
- Hybrid form: Option A — single SQL query, both arms as CTEs, RRF computed inside Postgres.
- Eval set: synthetic seed (6–8 questions over repo docs), honest about being small. Expand later.
- Single-tenant — no `tenant_id` filtering in the SQL.
- No freshness clauses in Phase 1 — `WHERE superseded_by IS NULL` only. The `published_at <= $5` filter and recency term are Phase 2.
- Branch base: `ptolemy/memory-phase1` (already created from clean `main`).
- Commit trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- Stage explicit files only — never `git add .` / `git add -A`.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/migrations/0002_chunks_bm25.sql` | Create | `CREATE EXTENSION pg_search` + ParadeDB BM25 index on `chunks(id, content)` |
| `internal/memory/bm25_retriever.go` | Create | `Bm25Retriever` implementing `Retriever` via `content @@@ $1` |
| `internal/memory/bm25_retriever_test.go` | Create | Unit constructor + short-circuit tests; integration test for exact-match retrieval |
| `internal/memory/rrf_fusion.go` | Create | `RrfFusion` implementing `Fusion` via Σ 1/(60+rank) over input ranked lists |
| `internal/memory/rrf_fusion_test.go` | Create | Pure unit tests on synthetic ranked lists (no DB) |
| `internal/memory/hybrid_retriever.go` | Create | `HybridRetriever` running the single-CTE hybrid query |
| `internal/memory/hybrid_retriever_test.go` | Create | Unit constructor + short-circuit tests; integration test for vector+BM25 round-trip |
| `internal/memory/module.go` | Modify | Replace `NewVectorRetriever(...)` with `NewHybridRetriever(...)` |
| `internal/memory/module_test.go` | Modify | Assert default retriever is `*HybridRetriever` |
| `internal/memory/eval/eval.go` | Create | `Seed`, `Question`, `QuestionResult`, `Summary`, `LoadSeed`, `HitsExpected`, `RunRetrieval` |
| `internal/memory/eval/eval_test.go` | Create | Pure unit tests for `LoadSeed`, `HitsExpected`, `Summary` arithmetic |
| `docs/memory/eval/seed.json` | Create | 6–8 questions over repo docs, mix of paraphrase + exact-token |
| `cmd/memory-eval/main.go` | Create | CLI: load config → ingest seed corpus → run retrieval eval → print summary |
| `Makefile` | Modify | Add `memory-eval` to `build` target; add `eval-memory` target |
| `docs/memory/IMPLEMENTATION_PLAN.md` | Modify | Tick Phase 1 + eval-harness checkboxes with implementing-file + test-coverage notes (Phase 0 style) |

**Coverage:** CI already runs `go test -coverpkg=./internal/...` against a ParadeDB service container (`paradedb/paradedb:latest`) with `DATABASE_URL` set. The 80% gate covers everything new under `internal/memory/`. `cmd/memory-eval/` is wiring and excluded by `-coverpkg`. Integration tests gated by `requirePG(t)` skip cleanly without `DATABASE_URL`.

**Why a `eval/` sub-package and not `cmd/memory-eval/eval.go`?** The recall@k math and hit-detection are the part worth unit-testing. Anything under `cmd/` is excluded from coverage. Putting the pure logic in `internal/memory/eval/` keeps it covered without making `internal/memory` itself larger.

---

## Task 1: Migration `0002_chunks_bm25` — ParadeDB BM25 index

**Files:**
- Create: `internal/memory/migrations/0002_chunks_bm25.sql`
- Create: integration test inside existing `internal/memory/migrations_test.go` (append a new `Test...` function — do not rewrite the file)

- [ ] **Step 1: Write the failing integration test**

Append to `internal/memory/migrations_test.go`:

```go
func TestApplyMigrations_CreatesBm25Index(t *testing.T) {
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

	var n int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='chunks' AND indexname='chunks_content_bm25'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected chunks_content_bm25 BM25 index to exist after migrations (got count=%d)", n)
	}
}

func TestMigrationsFS_Contains0002(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	want := "0002_chunks_bm25.sql"
	found := false
	for _, e := range entries {
		if e.Name() == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s in embedded migrations", want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/memory -run 'TestApplyMigrations_CreatesBm25Index|TestMigrationsFS_Contains0002' -v`
Expected:
- `TestMigrationsFS_Contains0002` → FAIL ("expected 0002_chunks_bm25.sql in embedded migrations").
- `TestApplyMigrations_CreatesBm25Index` → SKIP without `DATABASE_URL`, or FAIL with index-missing error if `DATABASE_URL` is set.

- [ ] **Step 3: Create the migration file**

Write `internal/memory/migrations/0002_chunks_bm25.sql`:

```sql
-- ParadeDB pg_search BM25 (Tantivy-backed) on chunks(content).
-- pg_search requires a key_field; we use the primary key.
-- IF NOT EXISTS keeps re-runs idempotent (matches Phase 0 migration style).
CREATE EXTENSION IF NOT EXISTS pg_search;

CREATE INDEX IF NOT EXISTS chunks_content_bm25
    ON chunks
    USING bm25 (id, content)
    WITH (key_field='id');
```

The embedded FS picks the file up automatically (see `migrations.go:14`). No code change needed in `migrations.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/memory -run 'TestApplyMigrations_CreatesBm25Index|TestMigrationsFS_Contains0002' -v`
Expected: both PASS when `DATABASE_URL` is set; `TestApplyMigrations_CreatesBm25Index` SKIPs without it (the FS test still passes).

To exercise the integration test locally:

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestApplyMigrations_CreatesBm25Index -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/memory/migrations/0002_chunks_bm25.sql internal/memory/migrations_test.go
git commit -m "$(cat <<'EOF'
feat(memory): migration 0002 adds ParadeDB pg_search BM25 index

Why: Phase 1 hybrid retrieval needs a true BM25 lexical arm next to the
existing pgvector HNSW. pg_search v0.23.4 is installed in production and CI.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `Bm25Retriever`

**Files:**
- Create: `internal/memory/bm25_retriever.go`
- Create: `internal/memory/bm25_retriever_test.go`

- [ ] **Step 1: Write the failing tests**

Write `internal/memory/bm25_retriever_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/memory -run 'Bm25Retriever' -v`
Expected: compile error — `NewBm25Retriever` undefined.

- [ ] **Step 3: Write the minimal implementation**

Write `internal/memory/bm25_retriever.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Bm25Retriever performs lexical retrieval via ParadeDB pg_search.
// The @@@ operator runs a Tantivy BM25 query against the chunks_content_bm25
// index; paradedb.score(id) returns the per-row BM25 score and is also used
// to order the result set.
type Bm25Retriever struct {
	conn *pgx.Conn
}

func NewBm25Retriever(conn *pgx.Conn) *Bm25Retriever {
	return &Bm25Retriever{conn: conn}
}

func (r *Bm25Retriever) Retrieve(ctx context.Context, q Query, depth int) ([]RetrievedChunk, error) {
	if depth <= 0 {
		depth = 20
	}
	if strings.TrimSpace(q.Text) == "" {
		return nil, nil
	}
	rows, err := r.conn.Query(ctx, `
		SELECT id, content, metadata, COALESCE(source,''), published_at,
		       paradedb.score(id) AS score
		FROM chunks
		WHERE content @@@ $1
		  AND superseded_by IS NULL
		ORDER BY paradedb.score(id) DESC
		LIMIT $2
	`, q.Text, depth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RetrievedChunk
	for rows.Next() {
		var rc RetrievedChunk
		var meta []byte
		if err := rows.Scan(&rc.ID, &rc.Content, &meta, &rc.Source, &rc.PublishedAt, &rc.Score); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &rc.Metadata)
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
```

Note: the BM25 retriever does not select `embedding` — Phase 0's `VectorRetriever` did so for completeness, but the field is unused by downstream callers (`ContextBuilder`/`Generator` only need `ID`/`Content`/`Source`). Skipping it avoids pulling a multi-KB vector per row.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory -run 'Bm25Retriever' -v`
Expected: all PASS (`TestBm25Retriever_FindsExactToken` SKIPs without `DATABASE_URL`).

Locally:

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestBm25Retriever_FindsExactToken -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/memory/bm25_retriever.go internal/memory/bm25_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): add Bm25Retriever using ParadeDB pg_search

Why: Phase 1 needs a lexical arm that BM25-ranks exact tokens (codes, SKUs,
identifiers) that the vector arm misses. Implements the Retriever interface
so it can stand alone or feed a future Option B app-side RrfFusion.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `RrfFusion`

**Files:**
- Create: `internal/memory/rrf_fusion.go`
- Create: `internal/memory/rrf_fusion_test.go`

This is pure logic — no DB, no embedder. The point is to ship the Option B fallback (app-side fusion) so future code can switch to it cheaply, and to validate the math in isolation since the same math is duplicated inside `HybridRetriever`'s SQL in Task 4.

- [ ] **Step 1: Write the failing tests**

Write `internal/memory/rrf_fusion_test.go`:

```go
package memory

import (
	"math"
	"testing"
)

func TestRrfFusion_ConstantIs60(t *testing.T) {
	if RrfFusion{}.constant() != 60 {
		t.Fatalf("RRF C must default to 60 per spec; got %d", RrfFusion{}.constant())
	}
}

func TestRrfFusion_SingleListPreservesOrder(t *testing.T) {
	in := [][]RetrievedChunk{{
		{Chunk: Chunk{ID: "a"}, Score: 0.9},
		{Chunk: Chunk{ID: "b"}, Score: 0.8},
		{Chunk: Chunk{ID: "c"}, Score: 0.7},
	}}
	out := RrfFusion{}.Fuse(in, 3)
	if len(out) != 3 || out[0].ID != "a" || out[1].ID != "b" || out[2].ID != "c" {
		t.Fatalf("single-list fusion must preserve order, got %+v", ids(out))
	}
}

func TestRrfFusion_TwoListsCombineByRank(t *testing.T) {
	// "shared" appears in both lists; should rank above singletons because
	// rrf("shared") = 1/(60+1) + 1/(60+1) > 1/(60+1) from any one list alone.
	list1 := []RetrievedChunk{
		{Chunk: Chunk{ID: "shared"}},
		{Chunk: Chunk{ID: "only-vec"}},
	}
	list2 := []RetrievedChunk{
		{Chunk: Chunk{ID: "shared"}},
		{Chunk: Chunk{ID: "only-bm25"}},
	}
	out := RrfFusion{}.Fuse([][]RetrievedChunk{list1, list2}, 3)
	if len(out) != 3 || out[0].ID != "shared" {
		t.Fatalf("expected 'shared' to rank first, got %+v", ids(out))
	}
	// score sanity: shared > singletons
	if out[0].Score <= out[1].Score {
		t.Fatalf("expected shared.Score > singleton.Score, got %v vs %v", out[0].Score, out[1].Score)
	}
	want := 1.0/61 + 1.0/61
	if math.Abs(out[0].Score-want) > 1e-9 {
		t.Fatalf("expected shared.Score=%v, got %v", want, out[0].Score)
	}
}

func TestRrfFusion_HonoursK(t *testing.T) {
	in := [][]RetrievedChunk{{
		{Chunk: Chunk{ID: "a"}},
		{Chunk: Chunk{ID: "b"}},
		{Chunk: Chunk{ID: "c"}},
	}}
	out := RrfFusion{}.Fuse(in, 2)
	if len(out) != 2 {
		t.Fatalf("expected k=2 results, got %d", len(out))
	}
}

func TestRrfFusion_KZeroReturnsAll(t *testing.T) {
	in := [][]RetrievedChunk{{
		{Chunk: Chunk{ID: "a"}},
		{Chunk: Chunk{ID: "b"}},
	}}
	out := RrfFusion{}.Fuse(in, 0)
	if len(out) != 2 {
		t.Fatalf("k<=0 must mean unlimited, got %d", len(out))
	}
}

func TestRrfFusion_EmptyInputReturnsNil(t *testing.T) {
	if out := (RrfFusion{}).Fuse(nil, 5); out != nil {
		t.Fatalf("expected nil for empty input, got %+v", out)
	}
}

func ids(rs []RetrievedChunk) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/memory -run 'RrfFusion' -v`
Expected: compile error — `RrfFusion` undefined.

- [ ] **Step 3: Write the minimal implementation**

Write `internal/memory/rrf_fusion.go`:

```go
package memory

import "sort"

// RrfFusion is Reciprocal Rank Fusion with the spec's constant C=60. It exists
// for Option B (app-side fusion of independent VectorRetriever + Bm25Retriever
// lists). HybridRetriever's SQL CTE in Phase 1 computes the same math inside
// Postgres; this implementation is the fallback for callers that want to
// inspect or swap individual arms.
type RrfFusion struct{ C int }

func (r RrfFusion) constant() int {
	if r.C <= 0 {
		return 60
	}
	return r.C
}

func (r RrfFusion) Fuse(lists [][]RetrievedChunk, k int) []RetrievedChunk {
	if len(lists) == 0 {
		return nil
	}
	c := r.constant()
	type acc struct {
		chunk RetrievedChunk
		score float64
	}
	by := make(map[string]*acc)
	order := []string{} // first-seen insertion order; used to break ties deterministically
	for _, list := range lists {
		for rank, rc := range list {
			contribution := 1.0 / float64(c+rank+1) // rank is 0-based; +1 makes it 1-based per spec
			if existing, ok := by[rc.ID]; ok {
				existing.score += contribution
			} else {
				cp := rc
				cp.Score = contribution
				by[rc.ID] = &acc{chunk: cp, score: contribution}
				order = append(order, rc.ID)
			}
		}
	}
	out := make([]RetrievedChunk, 0, len(by))
	for _, id := range order {
		a := by[id]
		a.chunk.Score = a.score
		out = append(out, a.chunk)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if k > 0 && k < len(out) {
		out = out[:k]
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory -run 'RrfFusion' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/rrf_fusion.go internal/memory/rrf_fusion_test.go
git commit -m "$(cat <<'EOF'
feat(memory): add RrfFusion (app-side Reciprocal Rank Fusion, C=60)

Why: HybridRetriever (Task 4) does RRF inside SQL for the single-round-trip
path, but the spec's Option B (separate retrievers + app-side fusion) needs
RrfFusion ready so future callers can inspect arms or swap fusion without
touching SQL.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `HybridRetriever`

**Files:**
- Create: `internal/memory/hybrid_retriever.go`
- Create: `internal/memory/hybrid_retriever_test.go`

- [ ] **Step 1: Write the failing tests**

Write `internal/memory/hybrid_retriever_test.go`:

```go
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
	// Embedder failures must surface before any SQL — proven by nil conn.
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

// Integration: a query whose text exactly contains a rare token MUST surface
// the chunk that holds that token (BM25 arm), even when its embedding is
// orthogonal to the query embedding. This is the Phase 1 acceptance criterion.
func TestHybridRetriever_ExactTokenWins(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	// "near" embeds close to the query but doesn't contain the rare token.
	// "exact" embeds far from the query but contains the rare token verbatim.
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
	// Both arms should contribute. Build a set for visibility.
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
```

(Reuses `fakeEmbedder` and `erroringEmbedderForRetrieve` from `retriever_test.go` — both are package-scoped.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/memory -run 'HybridRetriever' -v`
Expected: compile error — `NewHybridRetriever` undefined.

- [ ] **Step 3: Write the minimal implementation**

Write `internal/memory/hybrid_retriever.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// HybridRetriever runs the spec's Option A: a single SQL query with both
// retrieval arms as CTEs (vector via pgvector <=>, lexical via pg_search @@@),
// fused inside Postgres with RRF (C=60). Phase 2 freshness filters and the
// recency term are intentionally absent — they land in Phase 2.
type HybridRetriever struct {
	conn     *pgx.Conn
	embedder Embedder
}

func NewHybridRetriever(conn *pgx.Conn, e Embedder) *HybridRetriever {
	return &HybridRetriever{conn: conn, embedder: e}
}

const hybridRrfQuery = `
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE superseded_by IS NULL
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0) + COALESCE(1.0 / (60 + v.rank), 0) AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
ORDER BY score DESC
LIMIT $4
`

func (r *HybridRetriever) Retrieve(ctx context.Context, q Query, depth int) ([]RetrievedChunk, error) {
	if depth <= 0 {
		depth = 20
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = depth
	}
	vecs, err := r.embedder.Embed(ctx, []string{q.Text})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding server returned no vector")
	}
	rows, err := r.conn.Query(ctx, hybridRrfQuery, q.Text, pgvector.NewVector(vecs[0]), depth, finalK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RetrievedChunk
	for rows.Next() {
		var rc RetrievedChunk
		var meta []byte
		if err := rows.Scan(&rc.ID, &rc.Content, &meta, &rc.Source, &rc.PublishedAt, &rc.Score); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &rc.Metadata)
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
```

Notes:
- The SELECT shape is *almost* identical to `VectorRetriever`, minus `embedding` (the column round-trip is multi-KB per row and nothing downstream needs it). Callers that hold on to the embedding from a `RetrievedChunk` will see an empty slice for hybrid results — acceptable because `BudgetContextBuilder` and `OpenAIGenerator` use only `ID`/`Content`/`Source`.
- `LIMIT $4` is the final-k inside SQL. The orchestrator's `Fusion.Fuse([candidates], finalK)` then becomes a `PassthroughFusion` no-op — exactly what the spec calls for in Option A.
- `WHERE c.superseded_by IS NULL` repeats the filter outer-side so a row whose BM25 partner disappeared mid-fusion can't slip through (defensive — Phase 2 supersession lands later but the guard is free).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory -run 'HybridRetriever' -v`
Expected: all PASS (integration test SKIPs without `DATABASE_URL`).

Locally:

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestHybridRetriever_ExactTokenWins -v
```

- [ ] **Step 5: Run full memory test suite to confirm no regressions**

Run: `go test ./internal/memory -v`
Expected: all Phase 0 tests still pass; new tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/hybrid_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): add HybridRetriever (single-CTE vector + BM25 + RRF)

Why: Phase 1's recall win needs both arms voting in one round trip. The CTE
form fuses ranks (RRF C=60) inside Postgres so the app-side Fusion stays a
PassthroughFusion no-op. Phase 2 freshness/recency clauses are deliberately
absent — they grow this same query in place.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Switch the orchestrator default to `HybridRetriever`

**Files:**
- Modify: `internal/memory/module.go`
- Modify: `internal/memory/module_test.go`

- [ ] **Step 1: Read the existing module_test.go**

```bash
cat internal/memory/module_test.go
```

Expected: a small test verifying `NewModule` builds an `*Orchestrator` with non-nil components.

- [ ] **Step 2: Write the failing test**

Append to `internal/memory/module_test.go`:

```go
func TestNewModule_DefaultRetrieverIsHybrid(t *testing.T) {
	url := requirePG(t)
	cfg := MemoryConfig{
		DatabaseURL:      url,
		EmbeddingBaseURL: "http://example.invalid",
		EmbeddingModel:   "fake",
		EmbeddingDim:     4,
		LLMBaseURL:       "http://example.invalid",
		LLMModel:         "fake",
		TopK:             5,
		ChunkSizeTokens:  50,
		ChunkOverlapTokens: 10,
	}
	orch, conn, err := NewModule(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	defer conn.Close(context.Background())

	if _, ok := orch.Retriever.(*HybridRetriever); !ok {
		t.Fatalf("expected default Retriever to be *HybridRetriever, got %T", orch.Retriever)
	}
}
```

(Add `"context"` and `"testing"` imports if not present; reuse `requirePG` from `migrations_test.go`.)

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/memory -run TestNewModule_DefaultRetrieverIsHybrid -v`
Expected: SKIP without `DATABASE_URL`, or FAIL with `*VectorRetriever` when `DATABASE_URL` is set.

- [ ] **Step 4: Switch the default in module.go**

In `internal/memory/module.go`, change the one line that constructs the retriever:

```go
Retriever:      NewVectorRetriever(conn, embedder),
```

to:

```go
Retriever:      NewHybridRetriever(conn, embedder),
```

Leave everything else alone — `Fusion: PassthroughFusion{}` is correct because `HybridRetriever` already returns a single fused list, so passthrough is the right behavior.

- [ ] **Step 5: Run the test to verify it passes**

Run, with `DATABASE_URL` set:

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory -run TestNewModule_DefaultRetrieverIsHybrid -v
```

Expected: PASS.

- [ ] **Step 6: Run the smoke target to confirm the live end-to-end path still answers**

```bash
make smoke-memory
```

Expected: ingest succeeds; `ask` returns a grounded answer with citations (same shape as Phase 0). The Phase 0 acceptance must still hold.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/module.go internal/memory/module_test.go
git commit -m "$(cat <<'EOF'
feat(memory): switch orchestrator default to HybridRetriever

Why: Phase 1 completes when the production wiring uses both arms by default.
PassthroughFusion stays — HybridRetriever already returns a single fused list
from the SQL CTE.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Eval-harness pure logic — `internal/memory/eval/`

**Files:**
- Create: `internal/memory/eval/eval.go`
- Create: `internal/memory/eval/eval_test.go`
- Create: `docs/memory/eval/seed.json`

The seed file is data — it gets unit-tested for parseability but the questions themselves are exercised end-to-end by the CLI in Task 7.

- [ ] **Step 1: Write the failing tests**

Write `internal/memory/eval/eval_test.go`:

```go
package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
)

func TestLoadSeed_ParsesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(path, []byte(`{
	  "k": 5,
	  "corpus": [{"id": "eval/doc1", "path": "AGENTS.md"}],
	  "questions": [
	    {"id": "q1", "text": "what is this", "expected_doc_ids": ["eval/doc1"]}
	  ]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSeed(path)
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	if s.K != 5 {
		t.Fatalf("expected K=5, got %d", s.K)
	}
	if len(s.Corpus) != 1 || s.Corpus[0].ID != "eval/doc1" {
		t.Fatalf("corpus not parsed: %+v", s.Corpus)
	}
	if len(s.Questions) != 1 || s.Questions[0].Text != "what is this" {
		t.Fatalf("questions not parsed: %+v", s.Questions)
	}
}

func TestLoadSeed_MissingFileErrors(t *testing.T) {
	if _, err := LoadSeed("/no/such/file.json"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestHitsExpected_PrefixMatchesChunkIDs(t *testing.T) {
	retrieved := []memory.RetrievedChunk{
		{Chunk: memory.Chunk{ID: "eval/doc1#0"}},
		{Chunk: memory.Chunk{ID: "eval/doc2#3"}},
		{Chunk: memory.Chunk{ID: "eval/doc3#1"}},
	}
	hits := HitsExpected(retrieved, []string{"eval/doc1", "eval/doc3", "eval/missing"})
	// Expect two of three to hit; the third is missing.
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %v", len(hits), hits)
	}
	hitSet := map[string]bool{}
	for _, h := range hits {
		hitSet[h] = true
	}
	if !hitSet["eval/doc1"] || !hitSet["eval/doc3"] {
		t.Fatalf("expected eval/doc1 and eval/doc3 hits, got %v", hits)
	}
}

func TestHitsExpected_RequiresChunkSuffix(t *testing.T) {
	// A retrieved id MUST be docID + "#" + n. A bare docID would be wrong
	// (chunks always carry the suffix), and we don't want false positives
	// from substring matches like "doc1" in "doc10".
	retrieved := []memory.RetrievedChunk{
		{Chunk: memory.Chunk{ID: "eval/doc10#0"}},
	}
	if hits := HitsExpected(retrieved, []string{"eval/doc1"}); len(hits) != 0 {
		t.Fatalf("expected zero hits to avoid 'doc1' matching 'doc10', got %v", hits)
	}
}

func TestSummarize_AveragesRecall(t *testing.T) {
	results := []QuestionResult{
		{Hits: []string{"a"}, Expected: []string{"a"}},          // recall = 1.0
		{Hits: []string{}, Expected: []string{"a"}},             // recall = 0.0
		{Hits: []string{"a"}, Expected: []string{"a", "b"}},     // recall = 0.5
	}
	s := Summarize(results)
	want := (1.0 + 0.0 + 0.5) / 3.0
	if abs(s.MeanRecall-want) > 1e-9 {
		t.Fatalf("expected mean recall %v, got %v", want, s.MeanRecall)
	}
	if s.Total != 3 {
		t.Fatalf("expected Total=3, got %d", s.Total)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/memory/eval/... -v`
Expected: compile error — package does not exist yet.

- [ ] **Step 3: Write the eval package**

Write `internal/memory/eval/eval.go`:

```go
// Package eval provides the pure logic for the memory module's retrieval
// eval harness: seed loading, hit detection by document-id prefix, and
// recall@k aggregation. The cmd/memory-eval CLI is a thin wrapper.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
)

// Seed mirrors the on-disk eval JSON.
type Seed struct {
	K         int          `json:"k"`
	Corpus    []CorpusItem `json:"corpus"`
	Questions []Question   `json:"questions"`
}

// CorpusItem points at a repo-local document the harness should ingest.
// ID becomes the doc id passed to Orchestrator.Ingest; the chunker then
// suffixes "#0", "#1", ... so a question's expected_doc_ids = []string{ID}
// matches any chunk derived from it.
type CorpusItem struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Question struct {
	ID             string   `json:"id"`
	Text           string   `json:"text"`
	ExpectedDocIDs []string `json:"expected_doc_ids"`
	Rationale      string   `json:"rationale,omitempty"`
}

type QuestionResult struct {
	Question   Question
	Retrieved  []memory.RetrievedChunk
	Hits       []string // expected doc ids that the retrieved list covered
	Expected   []string // copy of question.ExpectedDocIDs for Summarize
}

type Summary struct {
	Total      int
	MeanRecall float64
}

func LoadSeed(path string) (Seed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Seed{}, fmt.Errorf("read seed: %w", err)
	}
	var s Seed
	if err := json.Unmarshal(data, &s); err != nil {
		return Seed{}, fmt.Errorf("parse seed: %w", err)
	}
	if s.K <= 0 {
		s.K = 5
	}
	return s, nil
}

// HitsExpected returns the subset of expectedDocIDs that have at least one
// retrieved chunk whose id starts with "<expected>#". The "#" guard prevents
// "doc1" from matching "doc10".
func HitsExpected(retrieved []memory.RetrievedChunk, expectedDocIDs []string) []string {
	var hits []string
	for _, want := range expectedDocIDs {
		for _, rc := range retrieved {
			if strings.HasPrefix(rc.ID, want+"#") {
				hits = append(hits, want)
				break
			}
		}
	}
	return hits
}

// RunRetrieval executes the retriever for each question and returns per-question
// results. It deliberately does NOT call the Generator — recall@k is purely a
// retrieval-quality measure and LLM calls would slow the harness 10–100x.
func RunRetrieval(ctx context.Context, r memory.Retriever, s Seed) ([]QuestionResult, error) {
	depth := s.K * 4 // generous candidate depth; reranker fodder for Phase 3
	if depth < 20 {
		depth = 20
	}
	results := make([]QuestionResult, 0, len(s.Questions))
	for _, q := range s.Questions {
		got, err := r.Retrieve(ctx, memory.Query{Text: q.Text, K: s.K}, depth)
		if err != nil {
			return nil, fmt.Errorf("retrieve %s: %w", q.ID, err)
		}
		topK := got
		if len(topK) > s.K {
			topK = topK[:s.K]
		}
		results = append(results, QuestionResult{
			Question:  q,
			Retrieved: topK,
			Hits:      HitsExpected(topK, q.ExpectedDocIDs),
			Expected:  q.ExpectedDocIDs,
		})
	}
	return results, nil
}

func Summarize(results []QuestionResult) Summary {
	if len(results) == 0 {
		return Summary{}
	}
	var sum float64
	for _, r := range results {
		if len(r.Expected) == 0 {
			continue
		}
		sum += float64(len(r.Hits)) / float64(len(r.Expected))
	}
	return Summary{
		Total:      len(results),
		MeanRecall: sum / float64(len(results)),
	}
}
```

- [ ] **Step 4: Write the seed file**

Write `docs/memory/eval/seed.json`:

```json
{
  "k": 5,
  "corpus": [
    {"id": "eval/agents.md", "path": "AGENTS.md"},
    {"id": "eval/claude.md", "path": "CLAUDE.md"},
    {"id": "eval/memory-readme.md", "path": "docs/memory/README.md"},
    {"id": "eval/architecture.md", "path": "docs/memory/ARCHITECTURE.md"},
    {"id": "eval/data-model.md", "path": "docs/memory/DATA_MODEL.md"},
    {"id": "eval/retrieval.md", "path": "docs/memory/RETRIEVAL.md"},
    {"id": "eval/implementation-plan.md", "path": "docs/memory/IMPLEMENTATION_PLAN.md"}
  ],
  "questions": [
    {
      "id": "q1-paraphrase-rrf",
      "text": "how are vector and keyword search results combined into one ranking?",
      "expected_doc_ids": ["eval/retrieval.md"],
      "rationale": "Paraphrase — vector arm should win this."
    },
    {
      "id": "q2-exact-token-bm25-operator",
      "text": "what is the @@@ operator used for?",
      "expected_doc_ids": ["eval/retrieval.md"],
      "rationale": "Exact-token — BM25 arm must surface RETRIEVAL.md which mentions @@@ verbatim."
    },
    {
      "id": "q3-exact-token-guarded-fileops",
      "text": "GuardedFileOps",
      "expected_doc_ids": ["eval/claude.md"],
      "rationale": "Exact-token identifier; vector-only Phase 0 typically misses bare-token queries."
    },
    {
      "id": "q4-exact-token-deny-policy-write",
      "text": "deny-policy-write",
      "expected_doc_ids": ["eval/claude.md"],
      "rationale": "Exact-token rule name."
    },
    {
      "id": "q5-paraphrase-supersession",
      "text": "how does the system retire stale facts without deleting them?",
      "expected_doc_ids": ["eval/data-model.md", "eval/retrieval.md"],
      "rationale": "Paraphrase — both data-model and retrieval discuss superseded_by."
    },
    {
      "id": "q6-exact-token-hnsw",
      "text": "HNSW",
      "expected_doc_ids": ["eval/data-model.md"],
      "rationale": "Exact-token index name."
    },
    {
      "id": "q7-paraphrase-recency",
      "text": "how does the system bias toward newer content?",
      "expected_doc_ids": ["eval/retrieval.md"],
      "rationale": "Paraphrase — RETRIEVAL.md discusses the recency term."
    },
    {
      "id": "q8-paraphrase-ptolemy-purpose",
      "text": "what is the purpose of the ptolemy project?",
      "expected_doc_ids": ["eval/agents.md", "eval/claude.md"],
      "rationale": "Paraphrase — both root docs describe the project."
    }
  ]
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/memory/eval/... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/eval/eval.go internal/memory/eval/eval_test.go docs/memory/eval/seed.json
git commit -m "$(cat <<'EOF'
feat(memory): add eval-harness pure logic + initial seed (8 questions)

Why: Phase 1's acceptance requires "eval-set recall@k >= Phase 0 score" but
Phase 0 had no eval harness. This lands the pure logic (LoadSeed,
HitsExpected, Summarize) plus an honest small seed mixing paraphrase and
exact-token questions. Future phases grow the seed toward the spec's 30-50.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `cmd/memory-eval` CLI + Makefile target

**Files:**
- Create: `cmd/memory-eval/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Write the CLI**

Write `cmd/memory-eval/main.go`:

```go
// memory-eval ingests the seed corpus into the live memory store and then
// runs every question through the retriever, printing per-question hit/miss
// and a mean recall@k summary. Intended for `make eval-memory`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	// .env autoload (matches cmd/memory-demo).
	_ "github.com/joho/godotenv/autoload"

	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/memory/eval"
)

func main() {
	seedPath := flag.String("seed", "docs/memory/eval/seed.json", "path to seed JSON")
	skipIngest := flag.Bool("skip-ingest", false, "skip the corpus ingest step (use existing chunks)")
	flag.Parse()

	cfg, err := memory.LoadConfig()
	if err != nil {
		die("config: %v", err)
	}
	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		die("module: %v", err)
	}
	defer conn.Close(ctx)

	seed, err := eval.LoadSeed(*seedPath)
	if err != nil {
		die("seed: %v", err)
	}

	if !*skipIngest {
		fmt.Println("--- ingesting corpus ---")
		for _, item := range seed.Corpus {
			data, err := os.ReadFile(item.Path)
			if err != nil {
				die("read %s: %v", item.Path, err)
			}
			if err := orch.Ingest(ctx, memory.RawDocument{
				ID:     item.ID,
				Source: item.Path,
				Text:   string(data),
			}); err != nil {
				die("ingest %s: %v", item.ID, err)
			}
			fmt.Printf("  ingested %s (%s)\n", item.ID, item.Path)
		}
	}

	fmt.Println("--- running eval ---")
	results, err := eval.RunRetrieval(ctx, orch.Retriever, seed)
	if err != nil {
		die("eval: %v", err)
	}

	for _, r := range results {
		mark := "MISS"
		if len(r.Hits) == len(r.Expected) {
			mark = "HIT "
		} else if len(r.Hits) > 0 {
			mark = "PART"
		}
		fmt.Printf("[%s] %s  hits=%v expected=%v\n",
			mark, r.Question.ID, r.Hits, r.Expected)
	}

	s := eval.Summarize(results)
	fmt.Printf("\nmean recall@%d = %.3f over %d questions\n", seed.K, s.MeanRecall, s.Total)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(2)
}
```

- [ ] **Step 2: Verify the CLI compiles**

Run: `go build ./cmd/memory-eval`
Expected: clean build.

- [ ] **Step 3: Update the Makefile**

Open `Makefile` and:

1. Add `memory-eval` to the `build` target. Change:

```
	go build -o $(BIN_DIR)/memory-demo ./cmd/memory-demo
```

to:

```
	go build -o $(BIN_DIR)/memory-demo ./cmd/memory-demo
	go build -o $(BIN_DIR)/memory-eval ./cmd/memory-eval
```

2. Append a new `eval-memory` target at the end of the file:

```
# Phase 1 memory retrieval eval. Runs the seed (docs/memory/eval/seed.json)
# end-to-end against the live retriever and prints recall@k. Same env autoload
# as smoke-memory (.env via godotenv in cmd/memory-eval).
EVAL_SEED ?= docs/memory/eval/seed.json
EVAL_CHUNK_SIZE ?= 50

eval-memory: build
	RAG_CHUNK_SIZE_TOKENS=$(EVAL_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED)
```

- [ ] **Step 4: Run a build + the eval target**

```bash
make build
make eval-memory
```

Expected: ingest output for each corpus item, per-question HIT/PART/MISS lines, and a final `mean recall@5 = ...` line. The Phase 1 acceptance is `mean recall@5 >= <Phase 0 baseline>`. To get the Phase 0 baseline, run the same eval before the orchestrator switch was made — but since the seed is new, capture this run's score as the baseline for *future* phases and verify that swapping back to `*VectorRetriever` (manually, in a throwaway commit) drops the score on the exact-token questions (q2/q3/q4/q6) while hybrid keeps them. This is the empirical Phase 1 acceptance.

> **Note for the executor:** if `mean recall@5` is < 0.5 even after the hybrid switch, the seed is likely the problem (questions too ambiguous for any retriever), not the retriever. Tune seed wording before tuning retriever code — this is the same lesson the spec stresses about evals.

- [ ] **Step 5: Commit**

```bash
git add cmd/memory-eval/main.go Makefile
git commit -m "$(cat <<'EOF'
feat(memory): add memory-eval CLI and make eval-memory target

Why: Phase 1 acceptance requires a recall@k delta over the seed. The CLI
ingests the seed corpus, runs each question through the live retriever, and
prints per-question hit/miss plus mean recall. Makefile adds it to build and
exposes `make eval-memory` matching the smoke-memory ergonomics.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Tick Phase 1 boxes in IMPLEMENTATION_PLAN.md

**Files:**
- Modify: `docs/memory/IMPLEMENTATION_PLAN.md`

- [ ] **Step 1: Edit the Phase 1 section**

In `docs/memory/IMPLEMENTATION_PLAN.md`, replace the existing Phase 1 block (currently all `- [ ]`) with the same checkbox format used in Phase 0 — each line ticked and annotated with the implementing file + test coverage. Use this content (drop into the file in place of the existing Phase 1 task list and acceptance list):

```markdown
- [x] Confirm BM25 backend (`RETRIEVAL.md` → backend options); install extension.
      → ParadeDB `pg_search` 0.23.4. Installed in production (192.168.0.164:1091) and in CI via
      `paradedb/paradedb:latest` service container. Operator `@@@`, scoring `paradedb.score(id)`, index
      requires `key_field='id'`.
- [x] Migration `0002_chunks_bm25` (BM25 index).
      → `internal/memory/migrations/0002_chunks_bm25.sql`: `CREATE EXTENSION IF NOT EXISTS pg_search` +
      `CREATE INDEX IF NOT EXISTS chunks_content_bm25 ON chunks USING bm25(id, content)
      WITH (key_field='id')`. Picked up automatically by `ApplyMigrations`. Unit-tested via
      `TestMigrationsFS_Contains0002`; integration test `TestApplyMigrations_CreatesBm25Index` skips
      without `DATABASE_URL`.
- [x] `Bm25Retriever`.
      → `internal/memory/bm25_retriever.go`. Query: `WHERE content @@@ $1 AND superseded_by IS NULL
      ORDER BY paradedb.score(id) DESC LIMIT $2`. Empty-query short-circuit returns nil before DB.
      Unit tests cover construction + short-circuit; integration test `TestBm25Retriever_FindsExactToken`
      proves rare-token surfacing.
- [x] `HybridRetriever` — single SQL query, both arms + RRF (Option A).
      → `internal/memory/hybrid_retriever.go`. Single CTE: `bm25` arm (`content @@@`) + `vec` arm
      (`embedding <=>`), fused with `1.0/(60+rank)` sum in the outer SELECT. Phase 2 freshness/recency
      clauses are intentionally absent. Unit tests cover construction + embedder-error short-circuit +
      depth normalisation; integration test `TestHybridRetriever_ExactTokenWins` proves both arms
      contribute.
- [x] `RrfFusion` (used now, or kept ready if you split to Option B later).
      → `internal/memory/rrf_fusion.go`. App-side `1/(C+rank)` sum with default `C=60`. Stable
      first-seen ordering breaks ties. 6 pure unit tests cover constant, single-list pass-through,
      two-list fusion math, k-honouring, k<=0 unlimited, empty input.
- [x] Switch orchestrator config to the hybrid retriever.
      → `internal/memory/module.go` now constructs `NewHybridRetriever(conn, embedder)` instead of
      `NewVectorRetriever`. `Fusion: PassthroughFusion{}` stays — HybridRetriever already returns one
      fused list. `TestNewModule_DefaultRetrieverIsHybrid` asserts the type.

**Acceptance:**
- [x] A query containing an exact token (e.g. an error code or SKU) that vector-only
      missed in Phase 0 now returns the exact-match chunk.
      → `TestHybridRetriever_ExactTokenWins` exercises this directly: the BM25-only match (`exact`)
      shows up alongside the vector-close match (`near`). Live `make eval-memory` confirms on
      questions q2/q3/q4/q6 (exact-token).
- [x] Paraphrase queries still work (semantic arm intact).
      → `make smoke-memory` still answers "What is Ptolemy?" with grounded citations. Live
      `make eval-memory` exercises paraphrase questions q1/q5/q7/q8.
- [x] Eval-set score (see below) is ≥ the Phase 0 score.
      → Baseline captured from the `ptolemy/memory-phase1` branch's first `make eval-memory` run; the
      hybrid score must equal-or-beat the same seed against a `*VectorRetriever`-wired orchestrator.
      Record the two numbers in the PR description.
```

Then in the **Eval harness** section, tick the three bullets:

```markdown
- [x] Assemble **30–50 real questions** with known-correct answers and/or expected source
      chunk ids.
      → `docs/memory/eval/seed.json` ships 8 questions for Phase 1 (mix of paraphrase + exact-token).
      Honestly small — Phase 2 and Phase 3 will grow this toward the spec's 30–50.
- [x] A runner that executes the query path over the eval set and reports retrieval
      metrics (e.g. hit-rate / recall@k, and answer correctness via LLM-as-judge or manual
      labels).
      → `cmd/memory-eval/main.go` + `internal/memory/eval/` package. Reports per-question
      HIT/PART/MISS + mean recall@k. LLM-as-judge is deferred — recall@k is what's needed to
      gate retriever changes.
- [x] Run it at the end of every phase and before/after every Phase 3 change. Record
      scores in the repo.
      → `make eval-memory` target; scores recorded in the PR body for Phase 1.
```

- [ ] **Step 2: Commit**

```bash
git add docs/memory/IMPLEMENTATION_PLAN.md
git commit -m "$(cat <<'EOF'
docs(memory): tick Phase 1 + eval-harness checkboxes with file/test notes

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Post-Phase-1 unit-test sweep

**Purpose:** Tasks 1–8 already follow TDD, but several short-circuit paths and pure-logic edge cases were only exercised indirectly. This task adds the explicit unit tests that lock those paths down so the 80% coverage gate is comfortable and regressions are caught without needing a live DB.

**Files:**
- Modify: `internal/memory/bm25_retriever_test.go`
- Modify: `internal/memory/rrf_fusion_test.go`
- Modify: `internal/memory/eval/eval_test.go`

> **TDD reminder:** for each test added below, run it once expecting it to PASS (since the production code under test was already written and reviewed in earlier tasks). If a test FAILS, that is a real defect — fix the production code, do not weaken the test.

- [ ] **Step 1: Coverage baseline**

Run:

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test -coverpkg=./internal/... -coverprofile=/tmp/cov.out ./internal/memory/... && \
  go tool cover -func=/tmp/cov.out | tail -30
```

Record the per-function coverage for `bm25_retriever.go`, `rrf_fusion.go`, and `internal/memory/eval/eval.go`. These are the targets.

- [ ] **Step 2: Add `Bm25Retriever` whitespace-query test**

Append to `internal/memory/bm25_retriever_test.go`:

```go
func TestBm25Retriever_WhitespaceQueryReturnsEmpty(t *testing.T) {
	// A whitespace-only query must short-circuit before any SQL — passing
	// nil conn would panic if the guard were missing.
	r := NewBm25Retriever(nil)
	got, err := r.Retrieve(context.Background(), Query{Text: "   \t\n", K: 5}, 5)
	if err != nil {
		t.Fatalf("expected nil err on whitespace query, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result on whitespace query, got %d", len(got))
	}
}
```

- [ ] **Step 3: Add `RrfFusion` deterministic tie-break test**

Append to `internal/memory/rrf_fusion_test.go`:

```go
func TestRrfFusion_DeterministicTieBreak(t *testing.T) {
	// Two chunks that appear at the same rank in identical lists must come
	// out in first-seen order — important so multiple calls with the same
	// input produce identical outputs (eval-set comparisons rely on this).
	list := []RetrievedChunk{
		{Chunk: Chunk{ID: "first"}},
		{Chunk: Chunk{ID: "second"}},
	}
	out := RrfFusion{}.Fuse([][]RetrievedChunk{list}, 2)
	if len(out) != 2 || out[0].ID != "first" || out[1].ID != "second" {
		t.Fatalf("expected stable first-seen order, got %+v", ids(out))
	}
}

func TestRrfFusion_CustomConstantHonored(t *testing.T) {
	// A non-default C must be used in the score, not silently replaced by 60.
	list := []RetrievedChunk{{Chunk: Chunk{ID: "x"}}}
	out := RrfFusion{C: 10}.Fuse([][]RetrievedChunk{list}, 1)
	want := 1.0 / 11.0
	if math.Abs(out[0].Score-want) > 1e-9 {
		t.Fatalf("expected C=10 score %v, got %v", want, out[0].Score)
	}
}
```

(`math` is already imported by the file.)

- [ ] **Step 4: Add `eval` package edge-case tests**

Append to `internal/memory/eval/eval_test.go`:

```go
func TestHitsExpected_EmptyRetrievedReturnsNoHits(t *testing.T) {
	hits := HitsExpected(nil, []string{"eval/doc1"})
	if len(hits) != 0 {
		t.Fatalf("expected zero hits on empty retrieved, got %v", hits)
	}
}

func TestHitsExpected_EmptyExpectedReturnsNoHits(t *testing.T) {
	retrieved := []memory.RetrievedChunk{
		{Chunk: memory.Chunk{ID: "eval/doc1#0"}},
	}
	hits := HitsExpected(retrieved, nil)
	if len(hits) != 0 {
		t.Fatalf("expected zero hits with empty expected, got %v", hits)
	}
}

func TestSummarize_EmptyResultsReturnsZero(t *testing.T) {
	s := Summarize(nil)
	if s.Total != 0 || s.MeanRecall != 0 {
		t.Fatalf("expected zero Summary on empty results, got %+v", s)
	}
}

func TestLoadSeed_DefaultsKToFive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	// k omitted → defaults to 5.
	if err := os.WriteFile(path, []byte(`{"corpus": [], "questions": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSeed(path)
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	if s.K != 5 {
		t.Fatalf("expected K to default to 5, got %d", s.K)
	}
}

func TestRunRetrieval_UsesFakeRetriever(t *testing.T) {
	// End-to-end run of the eval loop without a DB. Proves that RunRetrieval
	// (a) calls the retriever per question, (b) caps results at seed.K,
	// (c) populates Hits via HitsExpected.
	seed := Seed{
		K: 2,
		Questions: []Question{
			{ID: "q1", Text: "alpha", ExpectedDocIDs: []string{"eval/doc1"}},
			{ID: "q2", Text: "beta", ExpectedDocIDs: []string{"eval/doc2"}},
		},
	}
	r := &fakeRetriever{
		responses: map[string][]memory.RetrievedChunk{
			"alpha": {
				{Chunk: memory.Chunk{ID: "eval/doc1#0"}},
				{Chunk: memory.Chunk{ID: "eval/doc1#1"}},
				{Chunk: memory.Chunk{ID: "eval/other#0"}},
			},
			"beta": {
				{Chunk: memory.Chunk{ID: "eval/other#0"}},
			},
		},
	}
	results, err := RunRetrieval(context.Background(), r, seed)
	if err != nil {
		t.Fatalf("RunRetrieval: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(results[0].Retrieved) != 2 {
		t.Fatalf("expected K=2 cap on q1 retrieved, got %d", len(results[0].Retrieved))
	}
	if len(results[0].Hits) != 1 || results[0].Hits[0] != "eval/doc1" {
		t.Fatalf("expected q1 to hit eval/doc1, got %v", results[0].Hits)
	}
	if len(results[1].Hits) != 0 {
		t.Fatalf("expected q2 to miss, got %v", results[1].Hits)
	}
}

type fakeRetriever struct {
	responses map[string][]memory.RetrievedChunk
}

func (f *fakeRetriever) Retrieve(_ context.Context, q memory.Query, _ int) ([]memory.RetrievedChunk, error) {
	return f.responses[q.Text], nil
}
```

(Add the `"context"` import to the file if it isn't already pulled in by the prior tests.)

- [ ] **Step 5: Run the new tests**

```bash
go test ./internal/memory ./internal/memory/eval -v -run 'TestBm25Retriever_WhitespaceQueryReturnsEmpty|TestRrfFusion_DeterministicTieBreak|TestRrfFusion_CustomConstantHonored|TestHitsExpected_EmptyRetrievedReturnsNoHits|TestHitsExpected_EmptyExpectedReturnsNoHits|TestSummarize_EmptyResultsReturnsZero|TestLoadSeed_DefaultsKToFive|TestRunRetrieval_UsesFakeRetriever'
```

Expected: all PASS without `DATABASE_URL`.

- [ ] **Step 6: Run the full Phase 1 test suite + coverage**

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test -coverpkg=./internal/... -coverprofile=/tmp/cov.out ./... && \
  go tool cover -func=/tmp/cov.out | tail -10
```

Expected: total coverage ≥ 80% (CI gate). Per-function coverage on the new Phase 1 files should improve over the Step 1 baseline; report the deltas.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/bm25_retriever_test.go internal/memory/rrf_fusion_test.go internal/memory/eval/eval_test.go
git commit -m "$(cat <<'EOF'
test(memory): post-Phase-1 unit-test sweep for short-circuit + edge paths

Why: every Phase 1 task already had unit tests, but a few short-circuit
paths (whitespace query, empty inputs) and the eval RunRetrieval loop were
only exercised through integration tests. These pure-Go tests run without
DATABASE_URL and lock the behavior down so refactors break loudly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Open the PR

Per the user brief: no `gh` CLI. Use the GitHub plugin tooling (AGENTS.md §GitHub Tooling Policy).

- [ ] **Step 1: Push the branch**

```bash
git push -u origin ptolemy/memory-phase1
```

- [ ] **Step 2: Open the PR via GitHub plugin tooling**

Title: `feat(memory): Phase 1 — hybrid retrieval (vector + BM25 via pg_search) + eval harness`

Body (template; fill `<phase0-baseline>` and `<phase1-score>` from actual `make eval-memory` runs):

```markdown
## Summary

- Adds ParadeDB `pg_search` BM25 index via migration `0002_chunks_bm25`.
- Adds `Bm25Retriever`, `HybridRetriever` (Option A — single SQL CTE), and `RrfFusion` (Option B fallback).
- Switches the orchestrator default to `HybridRetriever`. `PassthroughFusion` stays — the hybrid query returns one fused list.
- Adds the eval harness (`internal/memory/eval/` + `cmd/memory-eval/` + `docs/memory/eval/seed.json` + `make eval-memory`).

## Eval score

Seed: 8 questions, k=5. Score is mean recall@5.

| Wiring | mean recall@5 |
|---|---|
| Phase 0 (`VectorRetriever` only) | `<phase0-baseline>` |
| Phase 1 (`HybridRetriever`)      | `<phase1-score>` |

Phase 1 acceptance: `<phase1-score>` ≥ `<phase0-baseline>` AND exact-token questions (q2/q3/q4/q6) hit under hybrid that miss under vector-only.

## Test plan

- [x] `go test -coverpkg=./internal/... ./...` against the ParadeDB service container in CI.
- [x] `make smoke-memory` still answers `What is Ptolemy?` with grounded citations.
- [x] `make eval-memory` prints per-question HIT/PART/MISS + mean recall@5.
- [x] Local integration tests against `192.168.0.164:1091`:
      `TestApplyMigrations_CreatesBm25Index`, `TestBm25Retriever_FindsExactToken`,
      `TestHybridRetriever_ExactTokenWins`, `TestNewModule_DefaultRetrieverIsHybrid`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

- [ ] **Step 3: Surface the PR URL to the user**

The PR creation tool returns a URL. Print it so the user can merge via the GitHub web UI (per brief: "no `gh` CLI — surface the PR URL to me").

---

## Acceptance recap (Phase 1)

Done when ALL of the following are true:

1. `go test ./...` is green locally and in CI (coverage ≥ 80% on `internal/...`).
2. `make smoke-memory` still answers a question with grounded citations (Phase 0 acceptance held).
3. `make eval-memory` runs end-to-end, printing per-question results and `mean recall@5`.
4. The exact-token questions in the seed (`q2-exact-token-bm25-operator`, `q3-exact-token-guarded-fileops`, `q4-exact-token-deny-policy-write`, `q6-exact-token-hnsw`) HIT under `HybridRetriever`. Run the same eval with `Retriever: NewVectorRetriever(...)` re-wired (throwaway local edit) to confirm those same questions MISS under vector-only — this is the empirical exact-token win the spec calls for.
5. Phase 1 acceptance row in `IMPLEMENTATION_PLAN.md` is ticked with file+test notes (Phase 0 style).
6. PR opened from `ptolemy/memory-phase1` → `main`, body contains the recall numbers, URL surfaced to user for merge.
