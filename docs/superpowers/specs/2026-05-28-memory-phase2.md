# Memory Module — Phase 2 (Freshness) Design

**Date:** 2026-05-28
**Branch base:** `main` at `e7c1621` (after Phase 1 merge, PR #33).
**Work branch:** `ptolemy/memory-phase2`.
**Spec scope:** the *Freshness* layer per `docs/memory/IMPLEMENTATION_PLAN.md` §"Phase 2". Phase 2 makes Ptolemy prefer current content, retire stale facts, and answer point-in-time queries.

## Goal

Add three behaviors on top of the Phase 1 hybrid retriever, without rewriting the existing pipeline:

1. **Supersession** — a new ingest of a corrected document retires the chunks of the previous version so they stop being retrieved.
2. **Point-in-time queries** — `Query.AsOf` is honoured in the SQL, defaulting to `now()` when nil.
3. **Recency boost** — newer chunks rank slightly higher than older ones, all else equal.

The retrieval contract from Phase 1 stays intact: `Retriever.Retrieve(ctx, Query, depth) → []RetrievedChunk`. Existing callers compile unchanged.

## Locked decisions (from brainstorming)

| Decision | Choice | Reasoning |
|---|---|---|
| Supersession detection | **Explicit only.** Caller passes `RawDocument.Metadata["supersedes"] = "old-doc-id"`. | Matches Ptolemy's "fail-safe ask/oob" stance — no implicit replacements. No similarity-threshold magic, no source-key reuse hazard. Caller owns the assertion. |
| Supersession timing | **Synchronous at ingest, inside a Postgres transaction.** No `SupersessionJob` component. | The heavy `SupersessionJob` in `ARCHITECTURE.md` was sized for the embedding-similarity strategy (which we rejected). Explicit versioning is a single `UPDATE`; inline keeps consistency immediate and removes a scheduler concern. |
| Recency weight | **`0.1`** (spec default, hard-coded). | The spec's stated tuning knob; Phase 3 will revisit against an eval set that has fresh-vs-stale pairs. |
| Recency half-life | **`2592000` seconds (30 days)** (spec default, hard-coded). | Same justification — ship spec, measure later. |
| `as_of` SQL scope | **`published_at <= $5` only.** `valid_from`/`valid_to` window filter deferred (columns stay in schema, unused). | YAGNI — no chunk currently has the window columns populated, and the seed exercises no time-bound facts. |
| `as_of` CLI exposure | **No CLI flag** on `memory-demo` / `memory-eval`. | Integration tests cover acceptance #2. Add a flag later if a user-facing point-in-time query becomes a real need. |
| `RecencyTtlJob` | **Deferred to Phase 3.** | Ptolemy's content is evergreen reference docs; there is no concrete TTL policy yet. Spec's Phase 3 says "add enhancements one at a time, measure each." |
| `published_at` sourcing | **`RawDocument.Metadata["published_at"]` RFC3339, as Phase 0 already wired.** No front-matter parsers, no file-mtime fallback. | No current ingest path needs it. Add if/when a real loader does. |
| Eval-set extension | **None.** Acceptance #1 and #2 become integration tests; acceptance #3 is verified via the unchanged eval still scoring `mean recall@5 = 1.000`. | Synthetic freshness questions would measure code correctness, not retrieval quality — noise in the recall@k headline. Eval will gain real freshness pairs in Phase 3 alongside the recency tuning. |

## Architecture

Phase 2 is purely additive to the Phase 1 pipeline. Every change is one of: a new migration, a new method on an existing component, or a parameter added to an existing SQL constant.

### Migration `0003_chunks_freshness`

All freshness columns (`published_at`, `valid_from`, `valid_to`, `superseded_by`) already exist from Phase 0. This migration adds only the two supporting indexes:

```sql
-- Phase 2: support freshness filters cheaply.
CREATE INDEX IF NOT EXISTS chunks_published_at
    ON chunks (published_at);
CREATE INDEX IF NOT EXISTS chunks_live
    ON chunks (id) WHERE superseded_by IS NULL;
```

Picked up automatically by `ApplyMigrations` (which iterates embedded `migrations/*.sql` files in sorted order). Idempotent per the Phase 0/1 pattern.

### `Store` gains a transactional supersession method

The existing `Store` interface keeps `Upsert`, `Get`, `MarkSuperseded` (the 1:1 pointer used by future jobs). A new method handles the Phase 2 atomic write:

```go
type Store interface {
    Upsert(ctx context.Context, chunks []Chunk) error
    Get(ctx context.Context, ids []string) ([]Chunk, error)
    MarkSuperseded(ctx context.Context, oldID, newID string) error

    // SupersedeOnUpsert performs Upsert + UPDATE of all chunks belonging to
    // supersedesOldDocID inside a single transaction. The new chunks are
    // upserted first; then every existing chunk whose id starts with
    // "<supersedesOldDocID>#" and whose superseded_by IS NULL is pointed at
    // the representative new chunk (chunks[0].ID). If either step fails the
    // whole thing rolls back.
    SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error
}
```

The `PgStore` implementation wraps `pgx.Conn.BeginTx` + the two writes + `Commit` (or `Rollback` on error). The fake Store used in `orchestrator_test.go` gains a recorder field so unit tests can assert the call shape without a real DB.

**Representative new chunk:** `chunks[0].ID` (which is `<newDocID>#0` because `FixedSizeChunker` produces ids in order). The `superseded_by` column is a single TEXT pointer; the retrieval filter only cares whether it is NULL or not, so the choice of representative does not affect query results. Following the link (e.g. for a future "show me the current version") lands the caller on a valid current chunk of the new doc.

### `Orchestrator.Ingest` reads `Metadata["supersedes"]`

The orchestrator decides which Store method to call:

```go
// Pseudocode of the new branch in Ingest, after chunking + embedding.
supersedesID, ok := doc.Metadata["supersedes"].(string)
if ok && supersedesID != "" {
    return o.Store.SupersedeOnUpsert(ctx, chunks, supersedesID)
}
return o.Store.Upsert(ctx, chunks)
```

If `Metadata["supersedes"]` references a doc that has no chunks (typo or first-time ingest), `SupersedeOnUpsert` succeeds with zero rows updated — logged at info level via zerolog so operators can spot typos without a hard fail.

### `Orchestrator.Answer` resolves `AsOf`

```go
asOf := time.Now().UTC()
if q.AsOf != nil {
    asOf = *q.AsOf
}
// pass asOf to retriever via Query (mutate a local copy; do not edit caller's Query)
local := q
local.AsOf = &asOf
candidates, err := o.Retriever.Retrieve(ctx, local, depth)
```

The retriever receives a `Query` whose `AsOf` is always non-nil, simplifying SQL parameter passing.

### `HybridRetriever.hybridRrfQuery` grows in place

Phase 1's 4-param query becomes a 5-param query. The added clauses live exactly where `docs/memory/RETRIEVAL.md` says they should:

```sql
-- Params: $1 user text · $2 query embedding · $3 candidate depth
--         $4 final k    · $5 as_of timestamp
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
      AND published_at <= $5            -- NEW in Phase 2
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE superseded_by IS NULL
      AND published_at <= $5            -- NEW in Phase 2
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0)
     + COALESCE(1.0 / (60 + v.rank), 0)
     + 0.1 * exp(-extract(epoch FROM $5 - c.published_at) / 2592000)  -- NEW
        AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
  AND c.published_at <= $5              -- NEW (defensive; mirrors the CTEs)
ORDER BY score DESC
LIMIT $4
```

Constants `60` (RRF), `0.1` (recency weight), `2592000` (half-life seconds) are spec defaults; Phase 3 turns them into tuning knobs against the eval set.

### `Bm25Retriever` also gets the `as_of` filter

For symmetry — and so any future caller that uses `Bm25Retriever` standalone (Option B) gets the same point-in-time semantics:

```sql
SELECT id, content, metadata, COALESCE(source,''), published_at,
       paradedb.score(id) AS score
FROM chunks
WHERE content @@@ $1
  AND superseded_by IS NULL
  AND published_at <= $2                -- NEW in Phase 2 (depth becomes $3)
ORDER BY paradedb.score(id) DESC
LIMIT $3
```

`VectorRetriever` (Phase 0, still present as a fallback) is **not** touched — it's not on the default retrieval path after Phase 1, and Phase 2 explicitly does not grow code that the orchestrator no longer wires. If a future caller revives `VectorRetriever`, it can grow the same `published_at <= $N` clause then.

## Components touched (file-level summary)

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/migrations/0003_chunks_freshness.sql` | Create | Two `CREATE INDEX IF NOT EXISTS` statements. |
| `internal/memory/migrations_test.go` | Modify | Append `TestMigrationsFS_Contains0003` + integration test for the two indexes. |
| `internal/memory/store.go` | Modify | Add `SupersedeOnUpsert` to `Store` interface; implement on `PgStore` using `BeginTx`. |
| `internal/memory/store_test.go` | Modify | Add `TestPgStore_SupersedeOnUpsert_Transactional` integration. |
| `internal/memory/orchestrator.go` | Modify | `Ingest` branches on `Metadata["supersedes"]`; `Answer` resolves `AsOf` default. |
| `internal/memory/orchestrator_test.go` | Modify | Append two unit tests using the existing fake Store + fake Retriever. |
| `internal/memory/hybrid_retriever.go` | Modify | `hybridRrfQuery` grows the two `published_at <= $5` clauses + the recency term; signature passes `q.AsOf` as `$5`. |
| `internal/memory/hybrid_retriever_test.go` | Modify | Add `TestHybridRetriever_PointInTime` (acceptance #2) and `TestHybridRetriever_PrefersFreshOverStale` (acceptance #1). |
| `internal/memory/bm25_retriever.go` | Modify | One added `published_at <= $2` clause; depth becomes `$3`. |
| `internal/memory/bm25_retriever_test.go` | Modify | Add `TestBm25Retriever_PointInTime`. |
| `docs/memory/IMPLEMENTATION_PLAN.md` | Modify | Tick the Phase 2 checkboxes with implementing-file + test references (Phase 0/1 style). |

Two files explicitly **not** touched:
- `internal/memory/types.go` — `Query.AsOf *time.Time` was added in Phase 0.
- `internal/memory/rrf_fusion.go` — RRF math is unchanged in Phase 2.

## Data flow

### Ingest of a corrected document

```
RawDocument{
  ID:       "agents.md.v2",
  Source:   "AGENTS.md",
  Text:     <new content>,
  Metadata: {"supersedes": "agents.md.v1", "published_at": "2026-05-28T..."},
}
   │
   ▼
Orchestrator.Ingest
   ├─ Chunker.Chunk → ["agents.md.v2#0", "agents.md.v2#1", ...]
   ├─ Embedder.Embed → [v0, v1, ...]
   ├─ chunks[i].Embedding = vecs[i]
   │
   ▼ Metadata["supersedes"] is set →
Store.SupersedeOnUpsert(chunks, "agents.md.v1")
   ├─ BEGIN
   ├─ INSERT ... ON CONFLICT UPDATE × len(chunks)  (same as plain Upsert)
   ├─ UPDATE chunks SET superseded_by='agents.md.v2#0'
   │      WHERE id LIKE 'agents.md.v1#%' AND superseded_by IS NULL
   ├─ COMMIT (or ROLLBACK if either step errored)
   ▼
return nil  (or wrapped error)
```

### Point-in-time + recency query

```
Query{Text: "...", AsOf: ptr(2025-12-01), K: 5}
   │
   ▼
Orchestrator.Answer
   ├─ asOf = *q.AsOf  (or time.Now().UTC() if nil)
   ├─ local := q ; local.AsOf = &asOf
   │
   ▼
HybridRetriever.Retrieve(ctx, local, depth=20)
   ├─ Embedder.Embed([q.Text]) → [vec]
   ├─ Postgres query (5 params: text, vec, depth, finalK, asOf)
   │     • bm25 CTE filters published_at <= asOf
   │     • vec  CTE filters published_at <= asOf
   │     • outer score adds 0.1 * exp(-(asOf - published_at)/2592000)
   ▼
[]RetrievedChunk (sorted by score desc, LIMIT finalK)
   │
   ▼
Fusion (passthrough) → ContextBuilder → Generator
```

## Error handling

| Failure | Where | Behaviour |
|---|---|---|
| `SupersedeOnUpsert` SQL error mid-tx | `PgStore` | `Rollback`, return wrapped error. Caller sees `ingest: supersede: <pgerr>`. Neither new chunks nor supersession persist. |
| `Metadata["supersedes"]` references a doc with no chunks | `PgStore` | UPDATE affects 0 rows. Not an error — info-level log via zerolog, ingest proceeds. Likely a typo OR a first-time ingest where the caller pessimistically named a predecessor. |
| `Metadata["supersedes"]` not a string | `Orchestrator.Ingest` | Type-assert with `ok`; if false, treat as not-set. Plain `Upsert` called. No error. |
| `Query.AsOf` in the future | SQL | `published_at <= $5` admits everything; recency term gives a slight positive boost (`exp(-negative-of-negative)` > 1). Acceptable; spec does not constrain. |
| `Query.AsOf` before any chunk's `published_at` | SQL | Empty result. Not an error — semantically correct ("no content existed yet"). |
| Migration `0003` runs against a DB without `0002`'s index | N/A | Sorted-order application means `0001`→`0002`→`0003`. A skipped or partial run is impossible because each is recorded in `memory_schema_migrations` only after success. |
| pgx connection drops mid-transaction | `PgStore.SupersedeOnUpsert` | `BeginTx` returns error or `Commit` fails → caller sees `ingest: supersede: <connerr>`. Idempotent ingest path means the caller can retry. |

## Testing strategy

TDD per CLAUDE.md. Each task lands red → green → commit. Every component has both unit and integration coverage.

### Unit tests (no DB; CI runs them without `DATABASE_URL`)

| Test | File | Asserts |
|---|---|---|
| `TestOrchestrator_Ingest_SupersedesOldDoc` | `orchestrator_test.go` | Given `Metadata["supersedes"]="old"`, the fake Store's `SupersedeOnUpsert` is called with `("old")`, `Upsert` is not. |
| `TestOrchestrator_Ingest_NoSupersedesNoOp` | `orchestrator_test.go` | Given no `supersedes` metadata, plain `Upsert` is called; `SupersedeOnUpsert` is not. |
| `TestOrchestrator_Answer_AsOfNilDefaultsToNow` | `orchestrator_test.go` | When `q.AsOf == nil`, the Query passed to the fake Retriever has `AsOf` populated and within a few seconds of `now()`. |
| `TestOrchestrator_Answer_AsOfRespected` | `orchestrator_test.go` | When `q.AsOf != nil`, the Query passed to the fake Retriever carries that same time. |
| `TestHybridRetriever_AsOfNilDefaultsToNow` | `hybrid_retriever_test.go` | Constructed via existing nil-conn pattern; embedder error short-circuit still works in 5-param form. |
| `TestMigrationsFS_Contains0003` | `migrations_test.go` | Embedded FS now contains `0003_chunks_freshness.sql`. |

### Integration tests (DB required; skip cleanly without `DATABASE_URL`)

| Test | File | Asserts |
|---|---|---|
| `TestApplyMigrations_CreatesFreshnessIndexes` | `migrations_test.go` | `chunks_published_at` and `chunks_live` exist in `pg_indexes` after `ApplyMigrations`. |
| `TestPgStore_SupersedeOnUpsert_Transactional` | `store_test.go` | Insert v1 (3 chunks), call `SupersedeOnUpsert(v2_chunks, "v1")`, assert all 3 old chunks have `superseded_by="v2#0"`. Then force a duplicate-key fault on the new chunks (e.g. with the same id) and verify rollback — none of v2 lands, v1 not supersededby. |
| `TestPgStore_SupersedeOnUpsert_NoOldDoc` | `store_test.go` | Calling with a `supersedesOldDocID` that has no matching rows succeeds, upserts the new chunks, supersedes nothing. |
| `TestHybridRetriever_PointInTime` | `hybrid_retriever_test.go` | Two chunks: one at `published_at = past`, one at `future`. Query with `AsOf` between → only past returned. **Acceptance #2.** |
| `TestHybridRetriever_PrefersFreshOverStale` | `hybrid_retriever_test.go` | Ingest v1 then v2 with `Metadata["supersedes"]=v1` via the full `Orchestrator.Ingest` path, query → v2 chunks returned, v1 absent. **Acceptance #1.** |
| `TestHybridRetriever_RecencyTermPresent` | `hybrid_retriever_test.go` | Two chunks identical except for `published_at` (1 day vs 60 days old), same text content, both BM25-hit. Asserts the newer one's `Score` is strictly greater than the older one's. |
| `TestBm25Retriever_PointInTime` | `bm25_retriever_test.go` | Same shape as the HybridRetriever variant. |

### End-to-end (live services; manual)

- `make smoke-memory` still produces a grounded answer. The smoke doc has no `Metadata["supersedes"]` so the supersession branch is not triggered; the published_at filter admits the just-ingested chunks; the recency term gives them a small positive boost.
- `make eval-memory` still reports `mean recall@5 = 1.000 over 8 questions`. **Acceptance #3 surrogate** — recency weighting did not degrade evergreen recall.

## Acceptance verification

| Spec acceptance | How verified |
|---|---|
| (1) Given an old and a corrected chunk for the same fact, retrieval returns the corrected one and not the stale one. | `TestHybridRetriever_PrefersFreshOverStale` integration test. End-to-end ingest path through Orchestrator + Store + retriever. |
| (2) A point-in-time query with a past `as_of` excludes content published after it. | `TestHybridRetriever_PointInTime` + `TestBm25Retriever_PointInTime` integration tests. |
| (3) Recency weighting measurably raises fresh results without tanking eval-set score. | Surrogate: `make eval-memory` still scores 1.000. Direct verification deferred to Phase 3, which will add fresh-vs-stale eval questions and tune the constants. |

## Out of scope (deferred to Phase 3 or later)

- `SupersessionJob` async component — explicit + synchronous made it redundant.
- `RecencyTtlJob` — no TTL policy for evergreen Ptolemy docs yet.
- Front-matter / file-mtime `published_at` parsers — Phase 0's metadata path covers callers that know the date.
- `valid_from` / `valid_to` window filtering — columns stay in schema; no use case yet.
- CLI `--as-of` flag on `memory-demo` / `memory-eval` — add when a real user wants point-in-time interactive queries.
- Reranker, query rewriting, RRF/recency tuning — explicit Phase 3 territory.
- `topic_digests` table and `DigestSynthesisJob` — Phase 3 optional, only with the spec's required conflict-resolution step.

## References

- [docs/memory/IMPLEMENTATION_PLAN.md](../../memory/IMPLEMENTATION_PLAN.md) §"Phase 2" — acceptance criteria.
- [docs/memory/RETRIEVAL.md](../../memory/RETRIEVAL.md) — the "query grows in place" SQL pattern.
- [docs/memory/DATA_MODEL.md](../../memory/DATA_MODEL.md) — column-level freshness contract.
- [docs/memory/ARCHITECTURE.md](../../memory/ARCHITECTURE.md) §"Async jobs" — `SupersessionJob` interface that this phase intentionally does **not** implement.
- [docs/superpowers/specs/2026-05-27-memory-phase0.md](2026-05-27-memory-phase0.md) — Phase 0 spec; defines the `Store` interface this phase extends.
