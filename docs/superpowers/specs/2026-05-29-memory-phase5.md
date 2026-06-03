# Memory Module — Phase 5 (GC Supersession + Dedup) Design

**Date:** 2026-05-29
**Branch base:** `main` at the commit that merges the Phase 4 PR (`ptolemy/memory-phase4`). **Do NOT branch from `ptolemy/memory-phase4`** — wait for the merge, then cut `ptolemy/memory-phase5` from clean `main`.
**Work branch:** `ptolemy/memory-phase5`.
**Spec scope:** the *correction/redundancy* layer on top of the Phase 4 lifecycle, per `docs/memory/PHASE_5_MEMORY_GC_SUPERSESSION_DEDUP.md` (bridge) and **`docs/memory/SPEC-GC.md` §5 (source of truth)** + §8/§9. Phase 5 makes the version chain the single supersession path, adds the zero-cost structured-fact ladder + the `confidence` news/verification flow at ingest, and adds a scope-gated trigram `dedupRecent()` pass to the sweep (shipped but gated off). The whole comparison path is **pure-DB — no embeddings, no LLM** (SPEC-GC §8/§9, non-negotiable).

> **Logistics note.** This spec is written during a brainstorming session that runs *before* the Phase 4 PR is merged. The user chose "brainstorm + spec now, defer branch + implementation". The spec file is committed as the **first commit on `ptolemy/memory-phase5`** once that branch is cut from post-merge `main` — not onto `ptolemy/memory-phase4` (which would pollute the open Phase 4 PR).

## Goal

Phase 4 gave the store a status lifecycle, reinforcement-on-read, observability, and a dormant sweep. It deliberately left the *correction/redundancy* machinery dormant: `status='superseded'`, `version`, `supersedes`, `confidence`, `fact_subject`, `fact_predicate` columns all exist but are unused, and `pg_trgm` is not installed. Phase 5 activates them:

1. **One supersession path.** Today two write paths (`SupersedeOnUpsert`, `MarkSuperseded`) set *only* `superseded_by`; `status`/`version`/`supersedes` are dormant. Phase 5 makes the version chain the single model: every supersession sets the full contract, retrieval filters on `status='active'` alone, and `History()` walks the chain.
2. **Zero-cost structured-fact ladder.** At ingest, when `fact_subject`+`fact_predicate` are both set, one indexed lookup decides duplicate (reinforce) vs supersession (version) — no comparison cost on the hot path otherwise.
3. **`confidence` news/verification flow.** First report stored `confidence='low'`; a verified `confidence='high'` correction supersedes it via the *same* path, original kept + linked.
4. **Scope-gated trigram dedup, shipped but gated off.** `dedupRecent()` in the sweep collapses *structurally-confirmed* near-duplicates within scope; everything ambiguous (including every contradiction) is kept. Gated behind `GC_DEDUP_ENABLED` (default false); redundancy is **measured on the eval set before** the gate is ever flipped.

The Phase 0–4 retrieval contract is preserved except for the single-filter switch in §"Read path", which is recall-neutral on the frozen eval seed. `make eval-memory` = **recall@5 1.000** must hold after migration 0005.

## Locked decisions (from brainstorming)

| Decision | Choice | Reasoning |
|---|---|---|
| **Supersession unification** | **Unify writes + single `status='active'` retrieval filter.** One tx helper sets old → `status='superseded'`+`superseded_by`; new → `supersedes`+`version+1`; audited. Both `Supersede()` (new, row-level) and `SupersedeOnUpsert()` (kept, doc-level) call it. Migration 0005 backfills `status='superseded' WHERE superseded_by IS NOT NULL`. Retrieval drops `superseded_by IS NULL`, keeps `status='active'` across **all** arms (incl. `VectorRetriever`). | Option A of three. The version chain is a superset of the Phase 2 `superseded_by` flag; one model, no parallel scheme (bridge §1, acceptance "one unified supersession path"). |
| **Dedup gating** | **Shipped but gated off** (`GC_DEDUP_ENABLED=false`). Redundancy measured on the eval set before enabling (acceptance #5). The contradiction-survival test runs regardless of the gate. | Same measure-then-keep discipline as Phase 3 recency tuning. Collapsing rows before redundancy is measured could silently cut recall. |
| **PR scope** | **One PR**, subagent-driven, strict TDD. | The brief's single-Phase-5 framing; the pieces share migration 0005 and the sweep. |
| **Phase 4 follow-ups folded in** | **Shutdown-race done-channel fix only.** NOT: `dead_at` partial index, GC-only config split, `boolEnv`/`durationEnv` warn. | The sweep is touched anyway, so the shutdown race is a natural fix. The others are YAGNI for this PR (dedup is gated off → few dead rows → no index pressure yet). |
| **`fact_*` / `confidence` population** | **Via `Metadata` at ingest** (`Metadata["fact_subject"]`, `["fact_predicate"]`, `["confidence"]`), mirroring `Metadata["scope"]`/`["supersedes"]`. `confidence` default `'normal'`. The structured ladder fires only when **both** `fact_*` are set. | Consistent with the established Phase 2/4 metadata pattern; no orchestrator-shape change. |
| **Fact "value"** | **No `fact_value` column.** The `(fact_subject, fact_predicate)` pair is the key; `content` is the value. Same pair + identical content = duplicate; same pair + different content = supersession. | YAGNI — a third column adds schema + index for no behavioral gain over comparing `content`. |
| **Eval fixtures** | **Separate from the recall@5 corpus.** Contradiction/duplicate/news pairs are dedicated test fixtures; the recall corpus (12 docs / 30 questions) is untouched. | Deliberately-duplicated docs in the recall corpus would muddy the 1.000 measurement and risk shifting the baseline. |
| **Dedup measurement** | **`eval-memory-dedup` harness mode** (mirrors `eval-memory-sweep`): dry-run `dedupRecent` over the frozen corpus, report would-collapse count + recall before/after, emit a verdict. | Reproducible artifact for acceptance #5 ("measured before enabled"). |
| **Sweep supersession pass** | **No separate supersession sweep pass.** Pass order = `dedupRecent` (gated) → `archiveDecayed` → `purgeDead`. Supersession is synchronous at ingest + the `Supersede()` API. | Deviation from SPEC §6's 4-pass list, surfaced + approved in brainstorming: the keep-both fallback makes an *automatic raw-text* supersession pass unsafe, and structured supersession already happens at ingest — nothing is left for a sweep pass to safely do. |
| **Dedup tuning defaults** | `GC_DEDUP_ENABLED=false`, `GC_DEDUP_THRESHOLD=0.7`, `GC_DEDUP_LOOKBACK=24h`. All config. | **Placeholders** per TASKS-GC; must be tuned against real volume via `Stats()` + the measurement mode. |

## Architecture

Phase 5 is additive to the Phase 4 pipeline: one migration, new `Store` supersession methods, a structured-fact branch in `Orchestrator.Ingest`, a single retrieval-filter switch, a new gated sweep pass, and a shutdown-race fix.

### Migration `0005_chunks_dedup_supersession`

`0005` follows `0004` in the embedded `migrations/*.sql` sorted order, recorded once in `memory_schema_migrations`. `pg_trgm` 1.6 is confirmed available on the ParadeDB instance (`pg_available_extensions`).

```sql
-- pg_trgm: deferred from Phase 4 (NOT enabled there, despite the bridge draft's claim).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Trigram GIN index for dedupRecent()'s similarity() / % candidate lookup.
CREATE INDEX IF NOT EXISTS chunks_content_trgm
    ON chunks USING gin (content gin_trgm_ops);

-- Structured-fact ladder lookup (the zero-cost step 1 of SPEC §5).
CREATE INDEX IF NOT EXISTS chunks_fact
    ON chunks (fact_subject, fact_predicate)
    WHERE fact_subject IS NOT NULL AND fact_predicate IS NOT NULL;

-- Unification backfill: bring any legacy Phase-2-superseded rows (superseded_by set,
-- status still 'active') onto the unified status model so the retrieval filter can
-- drop `superseded_by IS NULL` and rely on `status='active'` alone. Idempotent;
-- recorded once. No new status values (within the existing chunks_status_chk CHECK).
UPDATE chunks SET status = 'superseded'
 WHERE superseded_by IS NOT NULL AND status = 'active';
```

- **No `dead_at` partial index** (locked decision — dedup is gated off, so few dead rows; revisit when dedup is enabled at volume).
- The `version`/`supersedes`/`fact_*`/`confidence` columns already exist from migration 0004 — Phase 5 only adds indexes + the extension + the backfill, no `ALTER TABLE … ADD COLUMN`.
- `version` is **`INTEGER NOT NULL DEFAULT 1`** (confirmed in 0004; DATA_MODEL.md's "TEXT" note is stale — fix it as a doc touch-up).

### Store supersession methods (`store.go`)

A single private transactional helper is the one place the supersession column contract is written:

```go
// markSupersededTx retires oldIDs in favour of newID inside tx: sets each old row
// status='superseded', superseded_by=newID, and audits (active → superseded,
// 'supersession'). The caller is responsible for having inserted the new row(s) and
// for setting supersedes/version on them. Matching zero old rows is not an error.
func markSupersededTx(ctx context.Context, tx pgx.Tx, oldIDs []string, newID string) error
```

Public methods on `Store`/`PgStore`:

```go
// Supersede inserts newChunks (status='active'; the representative chunks[0] gets
// supersedes=oldID + version = old.version+1) and retires oldID via markSupersededTx,
// all in one transaction. Used by the structured-fact correction path. A slice (not a
// single Chunk) so a multi-chunk fact doc is handled; facts are usually single-chunk.
// Errors if oldID does not exist.
func (s *PgStore) Supersede(ctx context.Context, newChunks []Chunk, oldID string) error

// History returns the full version chain for id, oldest → newest, walking `supersedes`.
func (s *PgStore) History(ctx context.Context, id string) ([]Chunk, error)

// LookupFact returns the most-recent active chunk for (factSubject, factPredicate).
func (s *PgStore) LookupFact(ctx context.Context, factSubject, factPredicate string) (Chunk, bool, error)
```

`SupersedeOnUpsert` (doc-level, kept) is reworked to call `markSupersededTx` so the retired doc's chunks now also get `status='superseded'` + `version+1` (today it sets only `superseded_by`). `MarkSuperseded` (no production caller) is **removed** from the `Store` interface, `PgStore`, and the fake store; any test referencing it is updated to use `Supersede`.

`History` (SPEC §5):
```sql
WITH RECURSIVE history AS (
  SELECT * FROM chunks WHERE id = $1
  UNION ALL
  SELECT c.* FROM chunks c JOIN history h ON c.id = h.supersedes
)
SELECT * FROM history ORDER BY version;
```

### Read path: single `status='active'` filter

All retrieval arms drop `superseded_by IS NULL` and keep `status='active'`:

- `bm25_retriever.go` — one clause.
- `hybrid_retriever.go` — three spots (two CTEs + final join).
- `retriever.go` (`VectorRetriever`) — **currently has `superseded_by IS NULL` but NO `status='active'`** (a Phase 4 gap; it leaks archived/dead rows). Phase 5 switches it to `status='active'`, closing the gap.

Recall-neutral on the frozen seed (zero superseded rows there → `superseded_by IS NULL` was a no-op for it). The no-regression gate (`make eval-memory` = 1.000) proves it.

### Structured-fact ladder + confidence (`orchestrator.go`)

`Orchestrator.Ingest` reads three new metadata keys (default-safe):

```go
factSubject   := stringMeta(doc.Metadata, "fact_subject")   // "" if absent
factPredicate := stringMeta(doc.Metadata, "fact_predicate")
confidence    := stringMetaOr(doc.Metadata, "confidence", "normal")
```

`confidence` is threaded onto the chunk(s) and stored (CHECK `low|normal|high`). When **both** `fact_*` are set, the ladder runs before the plain `Upsert` (SPEC §5 step 1, ~0ms, one indexed lookup):

```
lookup := active chunk WHERE fact_subject=$1 AND fact_predicate=$2 (AND tenant matches)
  none                         → Upsert (normal insert)
  found, content identical     → Reinforce(found.ID)        # duplicate, no new row
  found, content differs       → Supersede(newChunk, found.ID)  # correction, version+1
```

The `confidence` news flow needs **no special logic**: a `confidence='high'` correction that differs from a stored `confidence='low'` fact takes the `Supersede` branch (or arrives via `Metadata["supersedes"]` → `SupersedeOnUpsert`); the original is kept, linked, and visible via `History`. A test asserts low→high keeps both walkable.

> The existing `Metadata["supersedes"]` doc-level branch in `Ingest` is unchanged in trigger; it now produces unified-model rows because `SupersedeOnUpsert` was reworked (above).

### Dedup pass: `dedupRecent()` (`sweep.go`), gated

```go
// dedupRecent collapses structurally-confirmed near-duplicates among active rows
// created within GC_DEDUP_LOOKBACK, within scope. No-op unless cfg.DedupEnabled.
// Contradictions and ambiguous pairs are ALWAYS kept (SPEC §5 safe fallback).
func (s *Sweeper) dedupRecent(ctx context.Context) error
```

**Two stages — the threshold only prefilters; collapse needs structural confirmation.** `GC_DEDUP_THRESHOLD` (trigram `similarity()`, GIN-indexed `%`) bounds *which pairs are even examined*; it is **not** the collapse trigger. Among the candidate pairs (active, same scope, `created_at >= now() - GC_DEDUP_LOOKBACK`, `similarity >= threshold`), a pair is collapsed **only** when structurally confirmed a duplicate. This is precisely how the cardinal rule ("a contradiction is NEVER collapsed") holds — trigram similarity *alone* never collapses anything:

| Pair signal (already a similarity candidate) | Action |
|---|---|
| both `fact_*` set, **same pair, normalized-equal content** | **collapse** → reinforce survivor; loser → `dead`/'duplicate' |
| both `fact_*` set, **same pair, content differs** | **NEVER collapse** — contradiction/supersession; keep both |
| **normalized-equal content** (raw text) | **collapse** (true restatement) |
| similar but **not normalized-equal** (raw text) | **keep both** (safe fallback) |

"Normalized-equal" = content equal after trimming + collapsing internal whitespace, **case-sensitive** (no case-folding — content can be code/identifiers where case is meaningful). The collapse bar is deliberately content *equality*, not a similarity cutoff: a contradiction such as "runs on port 8088" vs "runs on port 1089" is a high-similarity *candidate* but is **not** normalized-equal, so it is kept. Relaxing the collapse bar to a high-similarity (non-equal) cutoff is explicitly **deferred** — it would be a measured tuning decision via the dedup harness, not a Phase 5 default.

- **Survivor selection:** higher `access_count`, tie-break older `created_at` (the established row). Survivor is reinforced (`access_count+1`, `last_accessed_at=now()`).
- **Loser:** `status='dead'`, `dead_at=now()`, audited `chunk_audit(active → dead, 'duplicate')`. This is the **first real producer of `dead` rows**, so `purgeDead` becomes non-inert — but only when the gate is on.
- All writes are transactional + audited.

### Sweep order + shutdown-race fix (`sweep.go`, `module.go`, `cmd/workerd/main.go`)

`sweepOnce` order becomes: `dedupRecent` (gated) → `archiveDecayed` → `purgeDead`. No separate supersession pass (locked decision).

The shutdown race (Phase 4 follow-up): `MaybeStartSweep`'s stop func cancels the sweep context but does not wait for `Run()` to return before workerd closes the pgx conn. Fix:

```go
func (s *Sweeper) Run(ctx context.Context, done chan<- struct{}) {
    defer close(done)
    ...
}
// MaybeStartSweep's stop func: cancel(); then <-done (bounded by a short timeout) before
// the caller closes the conn.
```

`MaybeStartSweep` returns a stop func that cancels **and waits** on the done channel so workerd closes the connection only after the goroutine has exited.

### Config (`config.go` + `.env.example`)

`GCConfig` gains three knobs (placeholders, validated):

| Env | Type | Default | Purpose |
|---|---|---|---|
| `GC_DEDUP_ENABLED` | bool | `false` | gates the dedup sweep pass |
| `GC_DEDUP_THRESHOLD` | float | `0.7` | trigram `similarity()` **candidate-prefilter** cutoff (not the collapse trigger; see dedup §); validate in `(0,1]` |
| `GC_DEDUP_LOOKBACK` | duration | `24h` | "recent" window for dedup candidates; validate `≥ 1m` |

## Components touched (file-level summary)

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/migrations/0005_chunks_dedup_supersession.sql` | Create | `pg_trgm` + trigram GIN index + fact composite index + status backfill. |
| `internal/memory/migrations_test.go` | Modify | `TestMigrationsFS_Contains0005`; integration test asserts extension, both indexes, and the backfill (a synthetic `superseded_by`-set/`active` row becomes `superseded`). |
| `internal/memory/store.go` | Modify | `markSupersededTx`, `Supersede`, `History`; rework `SupersedeOnUpsert` onto the helper; **remove** `MarkSuperseded`. Scan `version`/`supersedes` where chunks are read for `History`. |
| `internal/memory/store_test.go` | Modify | Integration: supersede hides old / shows new / chain walkable / reversible; `SupersedeOnUpsert` now stamps status+version. |
| `internal/memory/store_unit_test.go` | Modify | Fake store records `Supersede`/`History`. |
| `internal/memory/retriever.go` | Modify | `VectorRetriever`: `superseded_by IS NULL` → `status='active'`. |
| `internal/memory/bm25_retriever.go` | Modify | Drop `superseded_by IS NULL`; keep `status='active'`. |
| `internal/memory/hybrid_retriever.go` | Modify | Same, three spots. |
| `internal/memory/hybrid_retriever_test.go` | Modify | Assert superseded row excluded by `status` alone. |
| `internal/memory/orchestrator.go` | Modify | `Ingest`: read `fact_subject`/`fact_predicate`/`confidence`; run the structured-fact ladder (Upsert / Reinforce / Supersede). |
| `internal/memory/orchestrator_test.go` | Modify | Ladder branches: duplicate→Reinforce, contradiction→Supersede, none→Upsert; confidence default + from metadata. |
| `internal/memory/sweep.go` | Modify | `dedupRecent` (gated); pass order; `Run` done-channel. |
| `internal/memory/sweep_test.go` | Modify | Contradiction-survival (gate on AND off), duplicate-collapse, dead/audit, gated no-op. |
| `internal/memory/config.go` | Modify | 3 dedup knobs + parsing + validation. |
| `internal/memory/config_test.go` | Modify | Defaults + rejects bad threshold/lookback. |
| `internal/memory/module.go` | Modify | `MaybeStartSweep` stop func waits on done. |
| `internal/memory/module_test.go` | Modify | stop func returns after Run exits (no leaked goroutine / use-after-close). |
| `cmd/workerd/main.go` | Modify | Use the waiting stop func before conn close. |
| `internal/memory/eval/` | Modify | `eval-memory-dedup` mode: dry-run dedup over frozen corpus, report would-collapse + recall before/after, verdict. |
| `internal/memory/eval/testdata/` | Create | Separate GC fixtures: contradiction pair, duplicate pair, news (low→high) — NOT in the recall corpus. |
| `Makefile` | Modify | `eval-memory-dedup` target (mirrors `eval-memory-sweep`). |
| `.env.example` | Modify | Document the 3 dedup env vars (placeholder note). |
| `docs/memory/DATA_MODEL.md` | Modify | Fix stale `version TEXT` → `INTEGER`; note 0005 indexes + extension; mark `fact_*`/`confidence`/`version`/`supersedes` live. |
| `docs/memory/TASKS-GC.md` | Modify | Tick Phases 4–5 supersession/structured/dedup items. |
| `docs/memory/IMPLEMENTATION_PLAN.md` | Modify | Add a Phase 5 section with file/test pointers. |

## Data flow

### Ingest (with the structured-fact ladder)

```
Orchestrator.Ingest(doc)
  ├─ read Metadata: scope, supersedes, fact_subject, fact_predicate, confidence
  ├─ Metadata["supersedes"] set?  → SupersedeOnUpsert(chunks, oldDocID)   [doc-level, unified]
  ├─ both fact_* set?             → ladder:
  │      lookup active (fact_subject, fact_predicate)
  │        none            → Upsert
  │        same content    → Reinforce(found.ID)        [duplicate]
  │        diff content    → Supersede(newChunk, found.ID)  [correction, vN+1]
  └─ else                         → Upsert
```

### Sweep (workerd goroutine, opt-in; dedup gated within it)

```
Sweeper.Run(ctx) [every GC_SWEEP_INTERVAL]      (defer close(done))
  └─ sweepOnce
       ├─ dedupRecent     (only if GC_DEDUP_ENABLED; scope-gated; contradictions kept)
       ├─ archiveDecayed  (scope='project' only; audited; global untouched)
       └─ purgeDead       (only if GC_PURGE_ENABLED; now non-inert once dedup produces dead rows)
  tick error → Error log, continue next interval

workerd shutdown: stop() → cancel(ctx); <-done (bounded) → close conn
```

## Error handling

| Failure | Where | Behaviour |
|---|---|---|
| Structured-fact lookup fails | `Orchestrator.Ingest` | Wrapped error returned from `Ingest` (ingest is not best-effort; a failed dedup decision must not silently drop or mis-store a fact). |
| `Supersede` tx fails | `store.go` | Rolled back (defer Rollback); error wrapped. Neither old nor new row is half-written. |
| `dedupRecent` tick errors | `Sweeper.Run` | Logged at Error; ticker continues (per-tick-tolerant, unchanged). |
| `GC_DEDUP_ENABLED=false` | `sweepOnce` | `dedupRecent` is a no-op; no candidate query, no writes. |
| Invalid dedup env (threshold ∉ (0,1], lookback < 1m) | `MemoryConfig` load | Fail-fast wrapped error (same pattern as existing knobs). |
| Migration 0005 partial failure | `ApplyMigrations` | Version not recorded → re-runs next start. `CREATE EXTENSION/INDEX IF NOT EXISTS` and the backfill `UPDATE` are all idempotent. |
| Sweep goroutine still running at shutdown | `MaybeStartSweep` stop func | Cancels ctx, waits on `done` (bounded) before conn close — no use-after-close. |

## Testing strategy

TDD per CLAUDE.md. Each task lands red → green → commit (`Co-Authored-By` trailer, explicit `git add`). Unit tests run without a DB; integration tests skip cleanly without `DATABASE_URL`.

### Unit (no DB)

| Test | File | Asserts |
|---|---|---|
| `TestGCConfig_DedupDefaults` | `config_test.go` | Unset → enabled false, threshold 0.7, lookback 24h. |
| `TestGCConfig_RejectsBadDedup` | `config_test.go` | threshold>1 / ≤0 / lookback<1m → error. |
| `TestOrchestrator_Ingest_FactDuplicate_Reinforces` | `orchestrator_test.go` | both `fact_*` set + existing same-content fact → `Reinforce`, no `Upsert`/`Supersede`. |
| `TestOrchestrator_Ingest_FactContradiction_Supersedes` | `orchestrator_test.go` | both `fact_*` set + existing diff-content fact → `Supersede`. |
| `TestOrchestrator_Ingest_NoFact_Upserts` | `orchestrator_test.go` | `fact_*` absent → plain `Upsert`. |
| `TestMaybeStartSweep_StopWaitsForRun` | `module_test.go` | stop func returns only after `Run` exits (done closed); no use-after-close. |
| `TestMigrationsFS_Contains0005` | `migrations_test.go` | Embedded FS contains `0005_chunks_dedup_supersession.sql`. |

### Integration (DB required)

| Test | File | Asserts |
|---|---|---|
| `TestApplyMigrations_0005DedupSupersession` | `migrations_test.go` | After migrate: `pg_trgm` installed, `chunks_content_trgm` + `chunks_fact` exist; a synthetic `superseded_by`-set/`status='active'` row is now `status='superseded'` (backfill). |
| `TestPgStore_Supersede_HidesOldShowsNew` | `store_test.go` | After `Supersede`: old `status='superseded'` (not retrieved), new active + retrieved, `superseded_by`/`supersedes`/`version+1` set, `chunk_audit` entry. |
| `TestPgStore_History_WalksChain` | `store_test.go` | `History` returns v1→v2→v3 ordered for a 3-deep chain. |
| `TestPgStore_Supersede_Reversible` | `store_test.go` | A single `UPDATE status='active'` on the old row (and clearing the new) restores retrievability. |
| `TestPgStore_SupersedeOnUpsert_StampsUnifiedModel` | `store_test.go` | Doc-level supersede now sets `status='superseded'`+`version+1`, not just `superseded_by`. |
| `TestHybridRetriever_ExcludesSupersededByStatusAlone` | `hybrid_retriever_test.go` | A `status='superseded'` row (with `superseded_by` NULL, to prove the status filter does the work) is excluded. |
| `TestSweeper_Dedup_ContradictionPairBothSurvive` | `sweep_test.go` | **Cardinal rule.** Insert two **active** rows directly (bypassing the ingest ladder) that are a high-similarity *candidate* but not normalized-equal — e.g. raw text "runs on port 8088" vs "runs on port 1089" — → `dedupRecent` (gate ON) → **both still active**, no `dead`. Proves trigram similarity alone never collapses. |
| `TestSweeper_Dedup_NearIdenticalCollapses` | `sweep_test.go` | Insert a normalized-equal pair (identical content, or differing only by whitespace) directly as two active rows (gate ON) → one survivor reinforced (`access_count` bumped), loser `status='dead'`/`dead_at` set + `chunk_audit('active','dead','duplicate')`. |
| `TestSweeper_Dedup_GatedOffIsNoOp` | `sweep_test.go` | `GC_DEDUP_ENABLED=false` → near-identical pair both survive untouched. |
| `TestSweeper_Confidence_NewsFlow` | `sweep_test.go` or `store_test.go` | low report → high correction supersedes → both linked, `History` walkable, retrieval shows only the high. |

### End-to-end (live services; manual)

- **No-regression gate:** `make eval-memory` = **recall@5 1.000 over 30 questions** after migration 0005 (the single-filter switch drops no active rows).
- **Acceptance #5 artifact:** `make eval-memory-dedup` reports would-collapse count + recall before/after over the frozen corpus; expected ≈ 0 collapses on the curated-distinct fixtures (if >0, that is the signal to keep the gate off).
- `make smoke-memory` still produces a grounded answer; ingest with `fact_*`/`confidence` metadata stores + resolves without error.

## Acceptance verification

| Bridge acceptance (`PHASE_5_*`) | How verified |
|---|---|
| Superseding hides old / shows new / chain walkable / reversible. | `TestPgStore_Supersede_HidesOldShowsNew`, `_History_WalksChain`, `_Supersede_Reversible`. |
| A contradiction pair is **never** collapsed (test inserts one, both survive). | `TestSweeper_Dedup_ContradictionPairBothSurvive` (gate on AND off). |
| Near-identical duplicates collapse to one reinforced survivor. | `TestSweeper_Dedup_NearIdenticalCollapses`. |
| One unified supersession path (no leftover parallel scheme). | `MarkSuperseded` removed; `SupersedeOnUpsert` reworked onto `markSupersededTx`; retrieval single-filter; `TestPgStore_SupersedeOnUpsert_StampsUnifiedModel`. |
| Dedup redundancy measured before the pass is enabled. | `make eval-memory-dedup` verdict; gate ships `false`. |
| Trigram similarity threshold is config. | `GC_DEDUP_THRESHOLD` + `TestGCConfig_*`. |
| No-regression: recall@5 still 1.000. | `make eval-memory` after 0005. |

## Out of scope (Phase 6 / later)

- **Conversational/project capture** (project-scoped ingest from chat) — Phase 6; the first real producer of `scope='project'` rows.
- **Decay-ranking-blend in retrieval** — Phase 6 (deferred per Phase 4 locked decision).
- **A separate supersession sweep pass** — explicitly not built (locked decision; the keep-both fallback makes it unsafe/unneeded).
- **`dead_at` partial index, GC-only config split, `boolEnv`/`durationEnv` warn** — deferred Phase 4 follow-ups, not folded into this PR.
- **Semantic/embedding dedup, LLM summarization/consolidation** — SPEC §9 out-of-scope; the comparison path stays pure-DB.
- **Reranker, query expansion, topic_digests** — still-deferred Phase 3 leftovers.

## References

- [docs/memory/PHASE_5_MEMORY_GC_SUPERSESSION_DEDUP.md](../../memory/PHASE_5_MEMORY_GC_SUPERSESSION_DEDUP.md) — bridge doc; build list + acceptance.
- [docs/memory/SPEC-GC.md](../../memory/SPEC-GC.md) — §5 (duplicates vs corrections, **source of truth**) + §8/§9 (non-negotiables).
- [docs/memory/TASKS-GC.md](../../memory/TASKS-GC.md) — GC build order, internal Phases 4–5.
- [docs/memory/DATA_MODEL.md](../../memory/DATA_MODEL.md) — the Phase 4 lifecycle columns Phase 5 activates.
- [docs/superpowers/specs/2026-05-28-memory-phase4.md](2026-05-28-memory-phase4.md) — Phase 4 design + its "Out of scope" Phase 5 handoff.
