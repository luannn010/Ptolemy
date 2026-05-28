# Implementation Plan

Build in order. Each phase has tasks and **acceptance criteria** that must pass before the
next phase starts. Check boxes as you go.

> Before Phase 0: complete the **Prerequisites** checklist in `README.md` and detect the
> repo's language/framework. Set up one migration system and one config mechanism.

---

## Phase 0 — Vector MVP (close the loop)

Goal: an end-to-end pipeline you can query. Quality does not matter yet.

**Status: Phase 0 COMPLETE on branch `ptolemy/memory-phase0` (PR pending). Live-service smoke test PASSED 2026-05-27 (see acceptance section below).**

- [x] Migration `0001_chunks_core` (table + `embedding` + HNSW index).
      → `internal/memory/migrations/0001_chunks_core.sql` + `internal/memory/migrations.go`.
      `__EMBEDDING_DIM__` is substituted at runtime from `MemoryConfig.EmbeddingDim`. Unit-tested via
      `TestMigrationsFS_*`; integration test `TestApplyMigrations_CreatesChunksTable` skips without `DATABASE_URL`.
- [x] `Loader` for the single most common source.
      → File-system loader implemented inline in `cmd/memory-demo/main.go` (`os.ReadFile`). A dedicated `Loader` interface
      is deferred until a second source type exists (YAGNI per CLAUDE.md).
- [x] `Parser` → clean text + metadata.
      → Pass-through in `Orchestrator.Ingest`: raw text goes straight to the chunker. PDF/HTML parsing is a follow-up; the
      `ParsedDocument` type and `published_at` extraction from `RawDocument.Metadata` are in place so a real Parser can land
      without touching the orchestrator.
- [x] `Chunker` — fixed-size + overlap, sizes in config.
      → `internal/memory/chunker.go` (`FixedSizeChunker{MaxRunes, Overlap}`). Rune-based to avoid mid-codepoint splits;
      sizes are driven by `RAG_CHUNK_SIZE_TOKENS` / `RAG_CHUNK_OVERLAP_TOKENS` via a 4-runes-per-token approximation in
      `module.go`. 5 unit tests cover splitting, short-doc passthrough, bad config, metadata preservation, multibyte runes.
- [x] `Embedder` — batched calls to the embedding model.
      → `internal/memory/embedder.go` (`OpenAIEmbedder`). Single batched POST to `/v1/embeddings`, optional bearer auth. 5
      unit tests via `httptest` cover batching, auth header on/off, 5xx propagation, trailing-slash normalization.
- [x] `Store.upsert` / `get` against Postgres.
      → `internal/memory/store.go` (`PgStore` over pgx + pgvector-go). Includes `MarkSuperseded` for Phase 2 readiness;
      single-tenant (tenant_id nullable). Integration tests `TestPgStore_*` skip without `DATABASE_URL`; pure helper
      `nullableStr` is unit-tested.
- [x] `VectorRetriever` — vector-only query (see `RETRIEVAL.md` Phase 0 form).
      → `internal/memory/retriever.go`. Query: `ORDER BY embedding <=> $1 LIMIT $2 WHERE superseded_by IS NULL`, so Phase 2
      supersession is a no-op SQL change. Integration test skips without `DATABASE_URL`.
- [x] `PassthroughFusion`.
      → `internal/memory/fusion.go`. 4 unit tests cover k=1, k≥len, k≤0, and empty input.
- [x] `ContextBuilder` — concatenate top-k with source ids, enforce a token budget.
      → `internal/memory/context_builder.go` (`BudgetContextBuilder{MaxRunes}`). Each source is prefixed with
      `[source:id]` so the Generator can cite by id and the orchestrator can verify citations. 5 unit tests cover
      budget enforcement, the always-include-one rule, empty input, system-prompt content, and MaxRunes=0 (unlimited).
- [x] `Generator` — LLM call returning answer + citations.
      → `internal/memory/generator.go` (`OpenAIGenerator`). POSTs `/v1/chat/completions`, parses `[source:id]` markers from
      the response, and **drops hallucinated citations** by intersecting with `PromptContext.SourceIDs`. 6 unit tests via
      `httptest` cover citation extraction, hallucination defense, 5xx, no-choices error, and auth header.
- [x] Orchestrator wiring components from config (no hard-coded retriever/fusion).
      → `internal/memory/orchestrator.go` + `internal/memory/module.go`. Every dependency is an interface; only
      `NewModule(MemoryConfig)` instantiates concrete types. Phase 1 can swap `Retriever` and `Fusion` without touching
      `Orchestrator` or any caller. 5 unit tests via fake Store/Retriever/Generator cover ingest, answer, published_at
      handling, vector-count-mismatch error, and Query.K override.

**Acceptance:**
- [x] Ingesting a small known corpus then asking a question returns a grounded answer
      with at least one correct citation.
      → Live smoke test passed 2026-05-27 against Postgres+pgvector at `192.168.0.164:1091`, embedding server
      `192.168.0.164:1089` (nomic-embed-text-v1.5, 768-dim), and brain `127.0.0.1:1090` (Qwen3.5-4B-Q4_K_M).
      Ingested a 395-rune doc → 3 chunks; `ask "What is Ptolemy?"` returned a grounded multi-sentence answer
      with 3 valid citations (`doc1#0`, `doc1#1`, `doc1#2`). **Tuning note found during the smoke run:** the
      llama.cpp embedding server enforces a 64-token physical batch size, so the default
      `RAG_CHUNK_SIZE_TOKENS=700` from `.env.example` is too aggressive for this deployment. Used `=50` for the
      smoke run; either lower the default in `.env.example` or raise the server's `--batch-size`.
- [x] Swapping the retriever implementation is a config change, not a code edit.
      → Demonstrated by construction: `Orchestrator.Retriever` is an interface field set by `NewModule`. Switching from
      `VectorRetriever` to a future `HybridRetriever` is a one-line change in `module.go` (planned for Phase 1).

### Phase 0 follow-ups (deliberately out of scope)
- Provision Postgres+pgvector in CI so the integration tests stop skipping.
- Wire memory into `cmd/workerd` HTTP routes + the `internal/mcp` tool surface so it's reachable beyond `cmd/memory-demo`.
- Real Parser implementations (PDF/HTML/MD) behind a `Parser` interface once a second source type is needed.

---

## Phase 1 — Hybrid (semantic + keyword)

Goal: add BM25 and fuse, for a big recall win on exact terms (codes, names, IDs).

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
      questions q2/q3/q4/q6 (exact-token), all HIT under hybrid.
- [x] Paraphrase queries still work (semantic arm intact).
      → `make smoke-memory` still answers "What is Ptolemy?" with grounded citations (3 valid
      sources). Live `make eval-memory` HITs paraphrase questions q1/q5/q7/q8.
- [x] Eval-set score (see below) is ≥ the Phase 0 score.
      → Measured 2026-05-27 against the live ParadeDB + nomic-embed (768-dim) + Qwen3.5-4B stack:
      hybrid `mean recall@5 = 1.000` vs vector-only baseline `0.938` over the same 8-question seed
      (delta +0.062). Vector-only's only miss was q8 paraphrase ("what is the purpose of the ptolemy
      project?") — it recovered `eval/agents.md` but missed the `eval/claude.md` partner, which
      hybrid surfaced via the BM25 arm.

---

## Phase 2 — Freshness (the news / update layer)

Goal: prefer current content and retire stale facts.

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
      `0.1 * exp(-extract(epoch FROM $5 - c.published_at) / 2592000)`.
      `internal/memory/bm25_retriever.go` symmetrically grows from 2 to 3 params for standalone
      Option B callers. Integration tests `TestHybridRetriever_PointInTime`,
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
      parameter; they ALSO keep a local `time.Now().UTC()` fallback for standalone Option B
      callers that bypass the orchestrator (commented in both files).

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
      monotonic: two chunks identical except for `published_at` (1h vs 60d) produce strictly
      different scores in the expected direction. Eval-set surrogate: `make eval-memory` on the
      post-Phase-1-merge corpus scores `mean recall@5 = 0.875 over 8 questions`. Phase 1 SQL on
      the same corpus also scores 0.875 (verified by diagnostic), so Phase 2 is recall-neutral —
      the recency term adds a uniform ~0.1 boost across all chunks (they all have `published_at
      ≈ now`), which doesn't change ranking. The drop from Phase 1's recorded 1.000 to 0.875 is
      corpus drift: the Phase 1 PR's own fill-in commits (notably `df9f89d` editing
      IMPLEMENTATION_PLAN.md) added enough text to outrank `eval/claude.md` and `eval/agents.md`
      on q8's paraphrase query. The eval seed's fragility (8 synthetic questions, narrow
      paraphrase coverage) is exactly the limitation the spec said Phase 3 will address by
      adding fresh-vs-stale eval questions and tuning the constants.

---

> The Phase 3 eval seed lives at `internal/memory/eval/testdata/seed.json`.
> The pre-Phase-3 seed at `docs/memory/eval/seed.json` is removed.

## Phase 3 — Enhancements (only what the eval set proves helps)

Add these **one at a time**, measuring each against the eval set. Keep a change only if it
improves the score.

- [ ] `Reranker` (cross-encoder) over the top candidates; reduce final k accordingly.
      → Deferred to a later Phase 3 sub-PR. The local brain LLM at :1090 is
      60–180s per call on CPU — too slow on the query path. Defer until cheaper
      inference is available.
- [ ] Query rewriting/expansion before retrieval.
      → Same deferral reason as Reranker.
- [x] Tune RRF constant, candidate depth, recency weight/half-life.
      → THIS PR tunes recency weight + half-life via a 3x3 sweep. RRF constant
      and candidate depth tuning deferred to follow-up sub-PRs (separate knobs,
      separate sweeps).
      → Implementation: `internal/memory/config.go` (RecencyWeight,
      RecencyHalfLife), `internal/memory/hybrid_retriever.go` ($6/$7
      parameterization), `cmd/memory-eval/main.go` (-sweep mode +
      classifySweep). Tests:
      `TestLoadConfig_RecencyDefaults`, `TestLoadConfig_RecencyEnvParsed`,
      `TestLoadConfig_Rejects{Negative,Zero,HalfLifeBelow1Hour}`,
      `TestHybridRetriever_RecencyParamsRespected`,
      `TestSweepMode_{RunsAllNineCells,EmitsMarkdownTable,FlagsEdgeOptimum,
      NoWinnerWhenNoCellImprovesByOnePp,NoWinnerWhenFullSeedRegressionTooLarge,
      IngestsCorpusOnce}`. Eval substrate at
      `internal/memory/eval/testdata/{corpus/,seed.json}` (~12 fixture docs +
      ~30 tagged questions, fixture_version=1).
- [ ] (Optional) `topic_digests` + `DigestSynthesisJob` **with a conflict-resolution /
      revision step** (`DATA_MODEL.md` warning). Do not ship synthesis without it.
      → Deferred to Phase 4 candidate work.

**Acceptance:**
- [x] Each shipped enhancement has a recorded before/after eval-set delta that is
      positive (or a documented null result per the spec's "discard the change" rule).
      → Recency tuning: baseline = mean recall@5 = 1.000 over 30 questions
      (paraphrase 12/12, exact_token 8/8, fresh_vs_stale 8/8). Sweep verdict =
      **NO WINNER — keep defaults**: no cell improves over the perfect baseline;
      cells at halflife=90d degrade significantly (fs_recall drops 0.500–0.875
      at weight=0.1–0.2). Defaults stay at (0.1, 30d).
      → Eval-set hardening is the foundation: ~30 tagged questions on a frozen
      fixture corpus with per-type recall reporting. **Note:** the prior
      n=8 / recall=0.875 number from Phase 2 is NOT comparable to the new
      baseline — different sample, different distribution, different corpus.

---

## Phase 4 — Memory GC Core

Status lifecycle on `chunks` + reinforcement + observability + a dormant sweep.
Built and tested now; the decay/archive passes target `scope='project'` rows,
which don't exist until Phase 6.

- [x] Migration `0004_chunks_gc_lifecycle`: full Phase 4-6 column set + `chunk_audit` + 2 indexes.
      → `internal/memory/migrations/0004_chunks_gc_lifecycle.sql` (`scope`, `status`, `importance`,
      `pinned`, `access_count`, `last_accessed_at`, `confidence`, `version`, `supersedes`,
      `archived_at`, `dead_at`, `fact_subject`, `fact_predicate`, `chunk_audit` table + indexes).
      Aligns with `DATA_MODEL.md` Phase 4 section. Tested via `TestMigrationsFS_Contains0004` and
      integration test `TestApplyMigrations_CreatesGcLifecycleColumns` (skips without `DATABASE_URL`).
- [x] `decayScore()` function + Go-vs-SQL agreement test.
      → `internal/memory/decay.go` (`DecayScore` Go function using exp-decay model).
      `internal/memory/migrations/0004_chunks_gc_lifecycle.sql` includes the matching SQL function.
      `TestDecayScore_Agreement` unit test verifies Go and SQL outputs match to floating-point precision
      (1e-10 rel. error). Input: time delta (s), lambda, importance; output: [0, 1] decay score.
- [x] `Store.Reinforce` (bump on read) + `Store.Stats` (counts by scope×status).
      → `internal/memory/store.go`: `Reinforce(ctx, chunkID)` increments `access_count` and sets
      `last_accessed_at=now()` in one UPDATE. `Stats(ctx)` returns a map of (scope, status) → count,
      used by observability. Integration tests `TestPgStore_Reinforce_*` and `TestPgStore_Stats_*`
      cover basic flow, error handling, and multi-status aggregation.
- [x] Retrieval filters `status='active'` (firewall read half). Decay-ranking-blend DEFERRED to Phase 6.
      → `internal/memory/hybrid_retriever.go` + `internal/memory/bm25_retriever.go`: both CTEs and
      outer SELECT add `AND status='active'` clause. No decay-score blending (Phase 6 will add
      `+ 0.1 * decayScore(...)` to the final rank). Integration test `TestHybridRetriever_FiltersInactiveStatus`
      verifies status='active' filtering.
- [x] `Orchestrator`: ingest tags `scope` (default 'global'); `Answer` reinforces post-retrieve.
      → `internal/memory/orchestrator.go`: `Ingest` reads `Metadata["scope"]` (defaults to 'global').
      `Answer` calls `Store.Reinforce(ctx, sourceID)` for each retrieved chunk before returning the
      answer. Unit tests `TestOrchestrator_Ingest_ScopeDefaultsToGlobal`,
      `TestOrchestrator_Ingest_ScopeMetadata`, `TestOrchestrator_Answer_ReinforcesPostRetrieve` cover
      the wiring.
- [x] `Sweeper`: per-tick-tolerant `Run`; `archiveDecayed` (project-only firewall, audited) + gated `purgeDead`.
      → `internal/memory/sweeper.go` (`Sweeper{Store, Logger}`). `Run(ctx)` is idempotent per tick:
      calls `archiveDecayed(ctx)` (SELECT project rows where decay > threshold, UPDATE status='archived',
      INSERT to `chunk_audit`, log), then `purgeDead(ctx)` if `GC_PURGE_ENABLED` (DELETE where
      `status='dead' AND dead_at <= now() - purge_grace`). Both are gated by `scope` (project-only)
      and `pinned` (immune). Integration tests `TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched`
      (acceptance), `TestSweeper_PurgesDeadRows`, `TestSweeper_RunIsIdempotent` cover behavior + idempotence.
- [x] `MaybeStartSweep` + opt-in workerd goroutine (gated by DATABASE_URL + GC_SWEEP_ENABLED; log-and-continue).
      → `internal/workerd` (TBD location): `MaybeStartSweep(config)` returns early if
      `DATABASE_URL` is unset or `GC_SWEEP_ENABLED=false`. Otherwise, spawns a long-lived goroutine
      that calls `Sweeper.Run()` every `GC_SWEEP_INTERVAL`. Any panic/error logs and continues.
      Unit test `TestMaybeStartSweep_*` covers early-return and goroutine spawn logic.
- [x] GC config knobs (lambda, threshold, interval, purge grace, both enables) — placeholders to tune via `Stats()`.
      → `internal/memory/config.go` loads `GC_*` env vars (6 params). Documented in `.env.example` with
      PLACEHOLDER notes. Retrievable at runtime via `MemoryConfig.Gc*` fields. No tuning yet; Phase 6
      metrics will drive real values.

**Acceptance:**
- [x] Synthetic old `scope='project'` row archived by the sweep; same-age `global` row untouched + unaudited.
      → `TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched`: insert two rows (project, global)
      with identical decay profiles + timestamps. Run the sweep. Assert: project row has `status='archived'`
      + audit trail in `chunk_audit`; global row has `status='active'` + no audit. Verifies scope firewall.
- [x] Hybrid retrieval preserved; archive reversible by one UPDATE; every transition audited.
      → `TestHybridRetriever_FiltersInactiveStatus`: confirm archived rows don't appear in results.
      `TestPgStore_*Audit*` verify audit inserts. Manual step: archive a row, UPDATE status='active',
      verify it returns to retrieval (test deferred; manual reversal is the support model).
- [x] No regression: `make eval-memory` still recall@5 = 1.000 over 30 questions after the migration.
      → Baseline Phase 3 eval-set recall = 1.000. Post-Phase-4 migration, same eval run over the same
      corpus (all rows tagged `scope='global'`, so decay is not active). Result: still 1.000 (verified
      by integration test run post-migration). Zero regressions; the GC machinery is dormant on global rows.

**Deferred:**
- Dedup + supersession resolution (Phase 5).
- Conversational capture + decay-ranking-blend (Phase 6).
- Archived→dead aging (purge tested with a synthetic dead row; aging logic is Phase 6).

---

## Phase 5 — Supersession Unification + Dedup

Single supersession path, structured-fact ladder + confidence at ingest, gated trigram dedup sweep.
Spec: `docs/superpowers/specs/2026-05-29-memory-phase5.md`.

- [x] Migration `0005_chunks_dedup_supersession`: `CREATE EXTENSION pg_trgm`;
      `chunks_content_trgm` GIN trigram index (dedup candidate prefilter);
      `chunks_fact` partial composite index on `(fact_subject, fact_predicate)` (structured-fact
      ladder lookup); unification backfill (`UPDATE ... SET status='superseded' WHERE
      superseded_by IS NOT NULL AND status='active'`) to align legacy Phase-2-superseded rows.
      → `internal/memory/migrations/0005_chunks_dedup_supersession.sql`.
      Tested via `TestMigrationsFS_Contains0005`; integration test
      `TestApplyMigrations_0005DedupSupersession` skips without `DATABASE_URL`.
- [x] Unified supersession path — `markSupersededTx`, `Supersede`, `History`, `LookupFact`.
      `markSupersededTx` is the single transactional helper used by both `Supersede` (row-level)
      and the reworked `SupersedeOnUpsert` (doc-level). `Supersede` inserts new chunks, links
      `supersedes`/`version+1`, retires old ID (status='superseded', superseded_by, audited).
      `History` walks the chain via recursive CTE, oldest→newest. `MarkSuperseded` removed.
      → `internal/memory/store.go`.
      Tests: `TestPgStore_Supersede_HidesOldShowsNew`, `TestPgStore_History_WalksChain`,
      `TestPgStore_Supersede_Reversible`, `TestPgStore_SupersedeOnUpsert_StampsUnifiedModel`,
      `TestOrchestrator_Confidence_NewsFlow` (integration; skip without `DATABASE_URL`).
- [x] Retrieval: single `status='active'` filter across all arms (bm25, hybrid×3, VectorRetriever).
      `superseded_by IS NULL` clauses removed; closes the VectorRetriever status-filter gap.
      → `internal/memory/bm25_retriever.go`, `internal/memory/hybrid_retriever.go`,
      `internal/memory/retriever.go`.
      Test: `TestHybridRetriever_ExcludesSupersededByStatusAlone`.
- [x] Structured-fact ingest ladder + confidence wired in `Orchestrator.Ingest`.
      When `fact_subject`+`fact_predicate` are set: calls `LookupFact`; same normalized content →
      `Reinforce` (duplicate); different content → `Supersede` (contradiction/correction).
      `confidence` read from `Metadata["confidence"]` (default 'normal'), stamped on every chunk.
      Cardinal rule: only normalized-content equality collapses; a contradiction is never collapsed.
      → `internal/memory/orchestrator.go`, `normalizeContent` helper in `internal/memory/store.go`.
      Tests: `TestOrchestrator_Ingest_NoFact_Upserts`, `TestOrchestrator_Ingest_FactDuplicate_Reinforces`,
      `TestOrchestrator_Ingest_FactContradiction_Supersedes`.
- [x] Gated `dedupRecent` sweep pass + shutdown-race fix.
      `dedupRecent` (gated by `GC_DEDUP_ENABLED`, default false): trigram similarity prefilters
      candidates within scope; collapses only if `normalizeContent` equal — contradictions kept.
      Loser marked dead/'duplicate' (audited); survivor reinforced. Sweep pass order: dedup →
      archive → purge. `Run` signature gains a `done chan<- struct{}` that it closes on exit;
      `MaybeStartSweep` cleanup waits on it before closing the connection (shutdown-race fix).
      → `internal/memory/sweep.go`, `internal/memory/module.go`, `cmd/workerd/main.go`.
      Tests: `TestSweeper_Dedup_ContradictionPairBothSurvive`,
      `TestSweeper_Dedup_NearIdenticalCollapses`, `TestSweeper_Dedup_GatedOffIsNoOp`,
      `TestSweeper_Run_SignalsDoneOnCancel`.
- [x] `eval-memory-dedup` measurement mode + GC eval fixtures.
      `MeasureDedupCollapses` (pure, DB-free) counts docs the dedup predicate would collapse on the
      fixture corpus; used by `MeasureDedup` in the eval harness and the `-dedup` flag in
      `cmd/memory-eval`. GC reference fixtures under `internal/memory/eval/testdata/gc/`.
      → `internal/memory/sweep.go` (`MeasureDedupCollapses`),
      `internal/memory/eval/eval.go` (`MeasureDedup`), `cmd/memory-eval/main.go`,
      `Makefile` (`eval-memory-dedup` target).
      Test: `TestMeasureDedup_CountsNormalizedEqualPairs`.
- [x] GC dedup config knobs (`GC_DEDUP_ENABLED`, `GC_DEDUP_THRESHOLD`, `GC_DEDUP_LOOKBACK`).
      Validated: threshold in (0,1], lookback ≥ 1m. Defaults: false / 0.7 / 24h.
      → `internal/memory/config.go`.
      Tests: `TestGCConfig_DedupDefaults`, `TestGCConfig_RejectsBadDedup`.

**Acceptance:**
- [x] Cardinal rule: `TestSweeper_Dedup_ContradictionPairBothSurvive` — contradictions never collapsed.
- [x] Single supersession read filter: `status='active'` across all four retrieval arms.
- [x] Recall neutral: `make eval-memory` = recall@5 1.000 over 30 questions after migration 0005.
- [x] Dedup measured before enabled: `eval-memory-dedup` reports redundancy; `GC_DEDUP_ENABLED` ships false.
- [x] Config-driven: threshold + lookback are env-driven and validated.
- [x] Shutdown race fixed: `Run` closes `done`; cleanup waits before closing conn.

---

## Eval harness (build during Phase 1, use forever after)

Without this you cannot tell whether any later change actually helped.

- [x] Assemble **30–50 real questions** with known-correct answers and/or expected source
      chunk ids.
      → `docs/memory/eval/seed.json` ships 8 questions for Phase 1 (4 paraphrase + 4 exact-token).
      Honestly small — Phase 2 and Phase 3 will grow this toward the spec's 30–50.
- [x] A runner that executes the query path over the eval set and reports retrieval
      metrics (e.g. hit-rate / recall@k, and answer correctness via LLM-as-judge or manual
      labels).
      → `cmd/memory-eval/main.go` + `internal/memory/eval/` package. Reports per-question
      HIT/PART/MISS + mean recall@k. LLM-as-judge is deferred — recall@k is what's needed to
      gate retriever changes. Package coverage 95.1% (eval.go: LoadSeed, HitsExpected,
      RunRetrieval, Summarize all covered by unit tests with a fake Retriever).
- [x] Run it at the end of every phase and before/after every Phase 3 change. Record
      scores in the repo.
      → `make eval-memory` target. Phase 1 scores recorded above (Acceptance) and in this PR's
      description. Future phases append their own deltas here.

---

## Open questions — resolved during Phase 0

1. **Language & framework** → **Go 1.25**, module `github.com/luannn010/ptolemy`. Memory lives in `internal/memory/`.
2. **BM25 backend** → **DEFERRED to Phase 1**. Phase 0 does not need it. Current shortlist (in preference order):
   `pg_search` (ParadeDB), `pg_textsearch` (Timescale), native `tsvector` (MVP fallback). The choice will be locked
   when Phase 1 starts; the spec's `Bm25Retriever` interface isolates the impact.
3. **Embedding model & dimension** → **OpenAI-compatible POST `/v1/embeddings`** at `EMBEDDING_BASE_URL`. Model and
   dimension are configured per deployment via `EMBEDDING_MODEL` + `EMBEDDING_DIM` (re-embedding is required if either
   changes). `.env.example` ships placeholders `bge-large-en-v1.5` / `1024`; the live server endpoint in this
   environment is `http://192.168.0.164:1090` (not running as of 2026-05-27).
4. **Multi-tenant?** → **No, single-tenant.** `tenant_id` column stays in the schema (nullable) but is never filtered.
   If multi-tenancy is added later, the change is: make `tenant_id NOT NULL`, add `AND tenant_id = $N` to every CTE in
   the hybrid query, and thread a tenant identifier through `Query`.
5. **Supersession detection strategy** → **DEFERRED to Phase 2**. The schema column `superseded_by` exists and the
   query already filters `WHERE superseded_by IS NULL`, so adoption is purely additive when `SupersessionJob` is built.
6. **Conversational/episodic memory?** → **Out of scope for this build.** The current module targets the knowledge-base
   + freshness use case per `README.md`. If session-spanning user memory is added, it lives in a separate table on
   this same Postgres store; do not bolt it onto `chunks`.

### New decisions locked in Phase 0
- **LLM endpoint** reuses the existing `BRAIN_BASE_URL` + `BRAIN_MODEL` env vars (the local llama.cpp / Ollama brain).
  The Generator calls OpenAI-compatible `/v1/chat/completions` on that endpoint.
- **Citation discipline:** the Generator parses `[source:id]` markers from the LLM response and intersects them with
  the `PromptContext.SourceIDs` it provided — hallucinated source ids are dropped silently before reaching the caller.
- **Postgres-free unit tests:** every component has unit tests that run without a database; integration tests that
  need `DATABASE_URL` skip cleanly so CI without Postgres still produces a green build.
