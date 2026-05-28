# Memory Module — Phase 4 (Memory GC Core) Design

**Date:** 2026-05-28
**Branch base:** `main` at `c848616` (after Phase 3 merge, PR #35).
**Work branch:** `ptolemy/memory-phase4`.
**Spec scope:** the *Memory GC Core* lifecycle layer per `docs/memory/PHASE_4_MEMORY_GC_CORE.md` (bridge) and `docs/memory/SPEC-GC.md` (generic source). Phase 4 gives the store a status lifecycle, reinforcement-on-read, observability, and a dormant background sweep — built and tested now, fully exercised once Phase 6 produces `project` rows.

## Goal

The store currently holds one species of data: evergreen reference chunks (`scope='global'`, implicitly). Phase 6 will add conversational memory (`scope='project'`) that *should* decay. Phase 4 lays the lifecycle machinery so those future entries are GC-managed from their first insert — no backfill later:

1. **Unified scope-tagged schema** — extend `chunks` (not a parallel `memories` table) with the full lifecycle column set, plus a `chunk_audit` trail.
2. **Reinforcement on read** — every retrieval bumps `access_count` + `last_accessed_at` so frequently-used memories resist decay.
3. **Observability** — a `Stats()` count by `(scope, status)` to tune the decay dials later.
4. **Background sweep** — a `time.Ticker` job (`archiveDecayed` + gated `purgeDead`) launched as an opt-in goroutine in `workerd`. The archive pass targets `scope='project'` only; `global` immunity is enforced by the **schema/query shape, not a tunable**.

The Phase 0–3 retrieval contract is preserved: the query "grows in place" by adding a single `AND status = 'active'` clause to each arm. Recall@5 = 1.000 must hold after the migration.

## Locked decisions (from brainstorming)

| Decision | Choice | Reasoning |
|---|---|---|
| Subsystem | **Memory GC Core** (the `PHASE_4_*` drafts), not the Phase 3 retrieval deferrals (reranker / query expansion / topic_digests). | Largest, best-specified next step; lays the lifecycle foundation Phase 6 depends on. |
| Table | **Extend `chunks`**, no parallel `memories` table. | The bridge doc's explicit instruction. `scope='global'` = the existing document KB; `scope='project'` = future conversational memory. |
| Schema scope | **Full set** — all Phase 4–6 columns in one migration (`scope, status, importance, pinned, access_count, last_accessed_at, confidence, version, supersedes, archived_at, dead_at, fact_subject, fact_predicate`) + `chunk_audit`. | Adding columns later is cheap, but adding-then-using is awkward when dormant columns (`fact_*`, `confidence`, `version`) shape index choices. One migration, zero rollbacks. |
| Backfill defaults | `scope='global'`, `status='active'`, `importance=1.0`, `pinned=false`, `access_count=0`, **`last_accessed_at=now()`**, `confidence='normal'`, `version=1`. Applied via column DEFAULTs. | Existing rows are the evergreen KB → `global` (decay-immune). `now()` chosen over `created_at` for migration simplicity; access-time signal is irrelevant for global rows (they never decay). |
| Audit table name | **`chunk_audit`** (renamed from the spec's generic `memory_audit`). | Matches the `chunks` table we extend. `chunk_id TEXT` FK-style reference. |
| Reinforce placement | **`Orchestrator.Answer`, after `Retrieve`, before `Fusion`.** One batched `UPDATE ... WHERE id = ANY($1)`. | Retriever stays pure (DB-free unit tests preserved). Single statement per query, cheap. |
| Decay-ranking-blend | **Deferred to Phase 6.** Phase 4 adds only `AND status='active'` to retrieval. | No `project` rows exist until Phase 6, so the decay-multiply would scale every current row by 1.0 — dead SQL that risks perturbing the recall@5=1.000 baseline for zero behavior change. |
| Sweep deployment | **Opt-in goroutine in `workerd`**, gated by `DATABASE_URL` present **AND** `GC_SWEEP_ENABLED=true`. Sweeper logic lives in `internal/memory` (unit-testable without workerd). | Keeps Ptolemy at one daemon. workerd today has no memory/Postgres wiring, so this adds a conditional load. |
| workerd-on-failure | **Log-and-continue, per-tick-tolerant.** A failed startup (Postgres unreachable while enabled) logs at Error and workerd proceeds without the sweep; a failed sweep *tick* logs at Error and the ticker continues to the next interval. Logs distinguish **disabled** (Info) from **enabled-but-failed** (Error). | The policy daemon's core job (HTTP + approvals) must survive a memory-DB outage. Degradation must be observable, not silently swallowed. |
| Purge grace anchor | **`dead_at TIMESTAMPTZ`** set on any `→dead` transition; `purgeDead` filters `status='dead' AND dead_at <= now() - grace`. | "Dead for 30d+" needs a real transition timestamp. Phase 4 has no automatic producer of `dead` rows (dedup is Phase 5); the purge pass is built + gated + tested with a synthetic dead row now. |
| Tuning defaults | `lambda=0.05`, archive threshold `0.1`, sweep interval `1h`, purge grace `30d`, `GC_PURGE_ENABLED=false`, `GC_SWEEP_ENABLED=false`. All config, no magic numbers in queries. | Per TASKS-GC: these are **placeholders** that must be tuned against real chat volume via `Stats()` once Phase 6 produces project rows. |

## Architecture

Phase 4 is additive to the Phase 3 pipeline. One migration, new lifecycle methods on `Store`, a single retrieval-clause addition, a new `Sweeper`, and a conditional workerd hook.

### Migration `0004_chunks_gc_lifecycle`

`0004` is free (the `topic_digests` migration that DATA_MODEL.md reserved for Phase 3 was never built). Picked up automatically by `ApplyMigrations` (embedded `migrations/*.sql`, sorted order, recorded in `memory_schema_migrations` — once-only, so plain `ADD CONSTRAINT` without `IF NOT EXISTS` is safe).

```sql
ALTER TABLE chunks
  ADD COLUMN IF NOT EXISTS scope            TEXT NOT NULL DEFAULT 'global',
  ADD COLUMN IF NOT EXISTS status           TEXT NOT NULL DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS importance       REAL NOT NULL DEFAULT 1.0,
  ADD COLUMN IF NOT EXISTS pinned           BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS access_count     INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS confidence       TEXT NOT NULL DEFAULT 'normal',
  ADD COLUMN IF NOT EXISTS version          INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS supersedes       TEXT,
  ADD COLUMN IF NOT EXISTS archived_at      TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS dead_at          TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS fact_subject     TEXT,
  ADD COLUMN IF NOT EXISTS fact_predicate   TEXT;

ALTER TABLE chunks
  ADD CONSTRAINT chunks_scope_chk      CHECK (scope IN ('project','global')),
  ADD CONSTRAINT chunks_status_chk     CHECK (status IN ('active','archived','superseded','dead')),
  ADD CONSTRAINT chunks_confidence_chk CHECK (confidence IN ('low','normal','high'));

CREATE INDEX IF NOT EXISTS chunks_status_active ON chunks (id)         WHERE status = 'active';
CREATE INDEX IF NOT EXISTS chunks_scope_status  ON chunks (scope, status);

CREATE TABLE IF NOT EXISTS chunk_audit (
    id         BIGSERIAL PRIMARY KEY,
    chunk_id   TEXT NOT NULL,
    old_status TEXT,
    new_status TEXT NOT NULL,
    reason     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chunk_audit_chunk_id ON chunk_audit (chunk_id);
```

- `superseded_by` already exists (Phase 2). `supersedes` (the back-pointer) and `version` ship now for Phase 5's supersession chain — dormant in Phase 4.
- `fact_subject`/`fact_predicate` columns ship now (full set) but their composite lookup index waits for Phase 5 (the structured-fact detection ladder that uses it).
- `pg_trgm` extension is **not** enabled here — only Phase 5 dedup needs it. YAGNI.
- Indexes added are exactly the two Phase 4 uses: the partial `status='active'` index (retrieval filter) and `(scope, status)` (the archive/purge passes + `Stats`).

### `Chunk` struct (`types.go`)

Gains fields mirroring the columns, leaving Phase 0–3 fields untouched:

```go
type Chunk struct {
    // ... existing Phase 0-3 fields ...
    Scope          string      // 'project' | 'global'
    Status         string      // 'active' | 'archived' | 'superseded' | 'dead'
    Importance     float64
    Pinned         bool
    AccessCount    int
    LastAccessedAt time.Time
    Confidence     string      // 'low' | 'normal' | 'high'
    Version        int
    Supersedes     *string
    ArchivedAt     *time.Time
    DeadAt         *time.Time
    FactSubject    *string
    FactPredicate  *string
}
```

### `decayScore()` (`score.go`)

Pure, DB-free, unit-testable. The single source of the decay formula in Go; the archive SQL inlines the identical expression, and a test asserts they agree (see Testing).

```go
// decayScore returns 1.0 for global/pinned rows (decay-immune by construction).
// Otherwise: importance * exp(-lambda * daysSinceAccess / (1 + accessCount)).
func decayScore(importance float64, scope string, pinned bool, accessCount int, daysSinceAccess, lambda float64) float64
```

### Store lifecycle methods (`store.go`)

```go
// Reinforce bumps access_count + last_accessed_at for the given ids in one
// statement. Not audited (hot path; auditing every read would explode chunk_audit).
func (s *PgStore) Reinforce(ctx context.Context, ids []string) error

// Stats returns counts grouped by (scope, status) for observability/tuning.
func (s *PgStore) Stats(ctx context.Context) ([]ScopeStatusCount, error)

type ScopeStatusCount struct {
    Scope  string
    Status string
    Count  int
}
```

The `Store` interface gains `Reinforce` and `Stats`; the fake store used in `orchestrator_test.go` records the calls.

### Read path: status filter (`hybrid_retriever.go`, `bm25_retriever.go`)

Both arms gain `AND status = 'active'` so archived/dead/superseded rows vanish from retrieval — the firewall's retrieval half. No other change; the recency term, RRF, and `$6/$7` knobs (Phase 3) are untouched. Example (HybridRetriever CTEs):

```sql
WHERE content @@@ $1
  AND superseded_by IS NULL
  AND status = 'active'          -- NEW in Phase 4
  AND published_at <= $5
```

The decay-ranking-blend is **deferred to Phase 6** (no project rows to act on; protects the recall baseline).

### `Orchestrator` wiring (`orchestrator.go`)

- **`Answer`:** after `Retriever.Retrieve` returns, collect the retrieved ids and call `Store.Reinforce(ctx, ids)` before `Fusion`. A reinforcement failure is logged but does **not** fail the query (reads must not break because a bookkeeping UPDATE failed).
- **`Ingest`:** read `Metadata["scope"]` (mirrors the Phase 2 `Metadata["supersedes"]` pattern), default `'global'`. Phase 4 ingests stay global; Phase 6 passes `'project'`.

### `Sweeper` (`sweep.go`)

```go
type Sweeper struct {
    conn *pgx.Conn
    cfg  GCConfig
}

// Run ticks every cfg.SweepInterval and calls sweepOnce. A failed tick is logged
// at Error and the loop continues to the next interval (per-tick-tolerant). Returns
// when ctx is cancelled.
func (s *Sweeper) Run(ctx context.Context)

// sweepOnce runs the Phase 4 passes in order: archiveDecayed, then purgeDead.
// Directly callable in tests (no ticker).
func (s *Sweeper) sweepOnce(ctx context.Context) error
```

**`archiveDecayed`** — archives `project` rows below the decay threshold, audited:
```sql
WITH moved AS (
  UPDATE chunks SET status='archived', archived_at=now()
   WHERE scope='project' AND status='active' AND NOT pinned
     AND importance * exp(-$1 * extract(epoch FROM now()-last_accessed_at)/86400
                          / (1 + access_count)) < $2
  RETURNING id
)
INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
SELECT id, 'active', 'archived', 'decay' FROM moved;
```
The `scope='project'` clause is the firewall — `global` rows are structurally unreachable. `$1=lambda`, `$2=threshold`.

**`purgeDead`** — the only destructive op. Deletes rows `dead` for ≥ `GC_PURGE_GRACE_DAYS`. Gated behind `GC_PURGE_ENABLED` (default false → no-op). Audited before delete:
```sql
-- only when GC_PURGE_ENABLED
WITH doomed AS (
  SELECT id FROM chunks
   WHERE status='dead' AND dead_at <= now() - ($1 || ' days')::interval
)
INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
SELECT id, 'dead', 'purged', 'purge' FROM doomed;
DELETE FROM chunks WHERE id IN (SELECT id FROM doomed);
```
Phase 4 has no automatic producer of `dead` rows (dedup is Phase 5); purge is built/gated/tested with a synthetic dead row now.

### Config (`config.go` + `.env.example`)

`GCConfig` is a sub-struct of `MemoryConfig` (or fields on it):

| Env | Type | Default | Purpose |
|---|---|---|---|
| `GC_SWEEP_ENABLED` | bool | `false` | workerd goroutine gate |
| `GC_SWEEP_INTERVAL` | duration | `1h` | ticker period |
| `GC_DECAY_LAMBDA` | float | `0.05` | decay rate |
| `GC_ARCHIVE_THRESHOLD` | float | `0.1` | archive below this score |
| `GC_PURGE_ENABLED` | bool | `false` | enables the destructive purge pass |
| `GC_PURGE_GRACE_DAYS` | float | `30` | min days `dead` before purge |

Validation: lambda ≥ 0, threshold in (0,1], interval ≥ 1m, grace ≥ 1 day. These are **placeholder defaults** — the spec and `.env.example` say so.

### `workerd` integration (`internal/memory` helper + `cmd/workerd/main.go`)

A testable helper keeps the conditional-load logic out of `main`:
```go
// MaybeStartSweep loads MemoryConfig; if DATABASE_URL is set AND GC_SWEEP_ENABLED,
// it connects, applies migrations, and starts the sweep goroutine. Returns a stop
// func (nil if disabled) and an enabled flag. Never panics.
func MaybeStartSweep(ctx context.Context) (stop func(), enabled bool, err error)
```
workerd calls it after existing wiring:
- disabled (no `DATABASE_URL` or `GC_SWEEP_ENABLED=false`) → `log.Info("memory sweep disabled")`, continue.
- enabled but connect/migrate fails → `log.Error(err)("memory sweep enabled but failed to start; continuing without it")`, continue.
- enabled + started → `log.Info("memory sweep started")`; on `<-stop`, cancel the sweep context.

## Components touched (file-level summary)

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/migrations/0004_chunks_gc_lifecycle.sql` | Create | Columns + constraints + 2 indexes + `chunk_audit`. |
| `internal/memory/migrations_test.go` | Modify | `TestMigrationsFS_Contains0004` + integration test asserting columns, `chunk_audit`, indexes, and **exact backfill default values** on a row. |
| `internal/memory/types.go` | Modify | Add lifecycle fields to `Chunk`. |
| `internal/memory/score.go` | Create | `decayScore()` pure function. |
| `internal/memory/score_test.go` | Create | Unit tests (global/pinned → 1.0; monotonic; reinforcement flattens) + Go-vs-SQL agreement test. |
| `internal/memory/store.go` | Modify | `Reinforce`, `Stats` on `Store` + `PgStore`; `ScopeStatusCount`. Scan lifecycle columns where rows are read. |
| `internal/memory/store_test.go` | Modify | Integration tests for `Reinforce`, `Stats`. |
| `internal/memory/store_unit_test.go` | Modify | Fake store records `Reinforce`/`Stats`. |
| `internal/memory/hybrid_retriever.go` | Modify | Add `AND status='active'` to both CTEs. |
| `internal/memory/hybrid_retriever_test.go` | Modify | `TestHybridRetriever_ExcludesNonActive`. |
| `internal/memory/bm25_retriever.go` | Modify | Add `AND status='active'`. |
| `internal/memory/orchestrator.go` | Modify | `Answer` calls `Reinforce` post-retrieve; `Ingest` reads `Metadata["scope"]`. |
| `internal/memory/orchestrator_test.go` | Modify | `TestOrchestrator_Answer_Reinforces`, `TestOrchestrator_Ingest_ScopeDefaultsGlobal`, `TestOrchestrator_Ingest_ScopeFromMetadata`. |
| `internal/memory/sweep.go` | Create | `Sweeper`, `Run`, `sweepOnce`, `archiveDecayed`, `purgeDead`, `GCConfig`. |
| `internal/memory/sweep_test.go` | Create | Firewall acceptance, archive reversibility, purge gating — integration. |
| `internal/memory/config.go` | Modify | GC knobs + env parsing + validation. |
| `internal/memory/config_test.go` | Modify | GC defaults + validation unit tests. |
| `internal/memory/module.go` | Modify | `MaybeStartSweep` helper. |
| `internal/memory/module_test.go` | Modify | `MaybeStartSweep` disabled-path test (no env → nil stop, enabled=false). |
| `cmd/workerd/main.go` | Modify | Call `MaybeStartSweep`; cancel on shutdown. |
| `.env.example` | Modify | Document the 6 GC env vars (placeholder note). |
| `docs/memory/IMPLEMENTATION_PLAN.md` | Modify | Add a Phase 4 section ticked with file/test pointers. |
| `docs/memory/DATA_MODEL.md` | Modify | Document the lifecycle columns + `chunk_audit`. |

## Data flow

### Read (with reinforcement)

```
Orchestrator.Answer(query)
   ├─ asOf resolve (Phase 2)
   ├─ Retriever.Retrieve → []RetrievedChunk     (SQL now filters status='active')
   ├─ Store.Reinforce(ids)                       NEW: batched UPDATE, log-only on error
   ├─ Fusion.Fuse
   ├─ ContextBuilder.Build
   └─ Generator.Generate
```

### Sweep (workerd goroutine, opt-in)

```
workerd main
   ├─ ... existing policy/HTTP wiring (SQLite) ...
   ├─ memory.MaybeStartSweep(ctx)
   │     ├─ DATABASE_URL unset OR GC_SWEEP_ENABLED=false → Info "disabled", return nil,false,nil
   │     ├─ connect+migrate fails → Error, return nil,true,err   (workerd continues)
   │     └─ ok → go Sweeper.Run(ctx); Info "started"; return cancel,true,nil
   └─ <-stop → cancel sweep ctx + shutdown HTTP servers

Sweeper.Run(ctx)  [every GC_SWEEP_INTERVAL]
   └─ sweepOnce
        ├─ archiveDecayed   (scope='project' only; audited; global untouched)
        └─ purgeDead        (only if GC_PURGE_ENABLED; dead_at <= now()-grace; audited)
      tick error → Error log, continue to next interval
```

## Error handling

| Failure | Where | Behaviour |
|---|---|---|
| `Reinforce` UPDATE fails | `Orchestrator.Answer` | Logged (warn); query still returns its answer. A bookkeeping failure must not break reads. |
| Sweep tick (`sweepOnce`) errors | `Sweeper.Run` | Logged at Error; ticker continues to next interval. Goroutine survives. |
| `GC_SWEEP_ENABLED=true` but Postgres unreachable | `MaybeStartSweep` (workerd) | Error log "enabled but failed to start; continuing without it"; workerd's HTTP + approvals stay up. |
| `GC_SWEEP_ENABLED` unset / `DATABASE_URL` missing | `MaybeStartSweep` | Info log "disabled"; no sweep; existing workerd behavior preserved. |
| Invalid GC env (lambda<0, threshold∉(0,1], interval<1m, grace<1d) | `MemoryConfig` load | Fail-fast wrapped error at config load (same pattern as existing knobs). |
| Migration `0004` partial failure | `ApplyMigrations` | Version not recorded → re-runs next start. `ADD COLUMN IF NOT EXISTS` is idempotent; `ADD CONSTRAINT` re-run after a recorded-failure would error, but the runner records only on full success, and constraints are added in a separate statement after columns — acceptable for a forward-only migration. |
| `purgeDead` with `GC_PURGE_ENABLED=false` | `sweepOnce` | Pass is a no-op; no deletes. |

## Testing strategy

TDD per CLAUDE.md. Each task lands red → green → commit. Unit tests run without a DB; integration tests skip cleanly without `DATABASE_URL`.

### Unit (no DB)

| Test | File | Asserts |
|---|---|---|
| `TestDecayScore_GlobalAndPinnedAreImmune` | `score_test.go` | `scope='global'` or `pinned` → exactly 1.0 regardless of age. |
| `TestDecayScore_DecaysWithAge` | `score_test.go` | For `project`, score strictly decreases as `daysSinceAccess` grows. |
| `TestDecayScore_ReinforcementFlattens` | `score_test.go` | Higher `access_count` → higher score for the same age (curve flattens). |
| `TestGCConfig_Defaults` | `config_test.go` | Unset env → lambda 0.05, threshold 0.1, interval 1h, grace 30, both enables false. |
| `TestGCConfig_RejectsBadValues` | `config_test.go` | lambda<0 / threshold>1 / interval<1m / grace<1d → error. |
| `TestMaybeStartSweep_DisabledWhenUnset` | `module_test.go` | No `DATABASE_URL` / `GC_SWEEP_ENABLED` unset → `(nil, false, nil)`. |
| `TestMigrationsFS_Contains0004` | `migrations_test.go` | Embedded FS contains `0004_chunks_gc_lifecycle.sql`. |

### Integration (DB required)

| Test | File | Asserts |
|---|---|---|
| `TestApplyMigrations_GCLifecycleColumns` | `migrations_test.go` | After migrate: all lifecycle columns exist, `chunk_audit` exists, both indexes exist, and a freshly-inserted row carrying only pre-GC columns reads back with **exact defaults** (scope='global', status='active', importance=1.0, pinned=false, access_count=0, confidence='normal', version=1, last_accessed_at within 5s of now). |
| `TestDecayScore_GoMatchesSQL` | `score_test.go` | Insert a `project` row with known `importance`/`access_count`/`last_accessed_at = now()-10d`. SQL computes `importance*exp(-lambda*epoch_days/(1+access_count))`; Go `decayScore` computes the same. Assert |sql−go| < 1e-6. Guards the formula duplication. |
| `TestPgStore_Reinforce_BumpsCounters` | `store_test.go` | `Reinforce([id])` increments `access_count` and advances `last_accessed_at`. |
| `TestPgStore_Stats_CountsByScopeStatus` | `store_test.go` | Insert known mix → `Stats` returns matching `(scope,status)` counts. |
| `TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched` | `sweep_test.go` | **Acceptance.** Insert old/unaccessed `project` row + same-age `global` row → `sweepOnce` → project row `status='archived'` with a `chunk_audit('active','archived','decay')` entry; global row still `active`, no audit row. |
| `TestSweeper_ArchiveIsReversible` | `sweep_test.go` | After archive, a single `UPDATE status='active'` restores retrievability. |
| `TestSweeper_PurgeDead_GatedAndGraced` | `sweep_test.go` | With `GC_PURGE_ENABLED=false`: synthetic `dead` row survives. With true + `dead_at` older than grace: row deleted + audited; a `dead` row newer than grace survives. |
| `TestHybridRetriever_ExcludesNonActive` | `hybrid_retriever_test.go` | An `archived` chunk is not returned by `Retrieve`; an `active` one is. |
| `TestOrchestrator_Answer_Reinforces` | `orchestrator_test.go` | Fake store records a `Reinforce` call with the retrieved ids (unit, fake store). |
| `TestOrchestrator_Ingest_ScopeDefaultsGlobal` / `_ScopeFromMetadata` | `orchestrator_test.go` | Ingest with no scope → chunks tagged `global`; `Metadata["scope"]="project"` → tagged `project`. |

### End-to-end (live services; manual)

- `make eval-memory` still reports **recall@5 = 1.000 over 30 questions** after the migration (the `status='active'` filter drops no active global rows). **No-regression gate.**
- `make smoke-memory` still produces a grounded answer; reinforcement runs on the read path without error.
- A manual `GC_SWEEP_ENABLED=true GC_SWEEP_INTERVAL=5s` workerd run logs "memory sweep started" then periodic ticks; without `DATABASE_URL` it logs "disabled" and serves normally.

## Acceptance verification

| Bridge acceptance (`PHASE_4_MEMORY_GC_CORE.md`) | How verified |
|---|---|
| Synthetic old project row archived by sweep; same-age global row untouched. | `TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched`. |
| Hybrid retrieval preserved (vector arm present); decay multiplies project rows only. | Vector arm unchanged; decay-multiply **deferred to Phase 6** (documented decision — no project rows yet). Status filter added to both arms. |
| Every status transition audited; archiving reversible by one UPDATE. | `chunk_audit` rows asserted in sweep tests; `TestSweeper_ArchiveIsReversible`. |
| Ingest/read latency matches pre-GC baseline; coverage gate held. | `make eval-memory` recall unchanged; one extra batched UPDATE on read; coverage ≥ 80%. |
| lambda, threshold, sweep interval, purge grace, GC_PURGE_ENABLED are config. | `GCConfig` + env + `TestGCConfig_*`. |

## Out of scope (Phase 5 / 6 / later)

- **Dedup pass** (pg_trgm `similarity()`) — Phase 5. `fact_*` columns + `pg_trgm` extension + composite index land then.
- **`Supersede()` / `History()` / structured-fact ladder / `confidence` news-flow** — Phase 5. `supersedes`/`version` columns ship now but are dormant.
- **Conversational/project capture** (project-scoped ingest from chat) — Phase 6. The first real producer of `scope='project'` rows.
- **Decay-ranking-blend in retrieval** — Phase 6 (deferred per locked decision).
- **archived→dead aging transition** — no Phase 4 pass moves rows to `dead`; the first producer is Phase 5 dedup. Purge is built/gated/tested with a synthetic dead row.
- **Reranker, query expansion, topic_digests** — still-deferred Phase 3 leftovers.

## References

- [docs/memory/PHASE_4_MEMORY_GC_CORE.md](../../memory/PHASE_4_MEMORY_GC_CORE.md) — bridge doc; placement + acceptance.
- [docs/memory/SPEC-GC.md](../../memory/SPEC-GC.md) — generic GC spec (source of the state machine + decay formula).
- [docs/memory/TASKS-GC.md](../../memory/TASKS-GC.md) — the GC build order this phase follows (its internal Phases 0–3).
- [docs/memory/DATA_MODEL.md](../../memory/DATA_MODEL.md) — chunk schema this phase extends.
- [docs/superpowers/specs/2026-05-28-memory-phase2.md](2026-05-28-memory-phase2.md) — the supersession/`superseded_by` groundwork this phase's lifecycle subsumes.
- [docs/superpowers/specs/2026-05-28-memory-phase3.md](2026-05-28-memory-phase3.md) — the recency/eval substrate this phase must not regress.

> The GC source drafts (`PHASE_4_MEMORY_GC_CORE.md`, `PHASE_5_*`, `PHASE_6_*`, `SPEC-GC.md`, `TASKS-GC.md`, `README-GC.md`) are currently untracked. The implementation plan's first task commits them as historical reference; **this spec is the authoritative Ptolemy-adapted design** where the two differ (e.g. `chunks`/`chunk_audit` not `memories`/`memory_audit`; ParadeDB `@@@`/pgvector not `ts_rank_cd`; decay-blend deferred).
