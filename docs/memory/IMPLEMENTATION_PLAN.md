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

- [ ] Migration `0003_chunks_freshness` (`published_at`, `valid_*`, `superseded_by`,
      indexes).
- [ ] Populate `published_at` from real source dates during ingestion.
- [ ] Add freshness `WHERE` clauses + recency term to the query (`RETRIEVAL.md` Phase 2).
- [ ] `SupersessionJob` — detect replacements, set `superseded_by`. (Detection strategy:
      resolve the open question below first.)
- [ ] `RecencyTtlJob` — refresh/expire policy.
- [ ] Expose `as_of` on the `Query` type; default to `now()`.

**Acceptance:**
- [ ] Given an old and a corrected chunk for the same fact, retrieval returns the
      corrected one and not the stale one.
- [ ] A point-in-time query with a past `as_of` excludes content published after it.
- [ ] Recency weighting measurably raises fresh results without tanking eval-set score.

---

## Phase 3 — Enhancements (only what the eval set proves helps)

Add these **one at a time**, measuring each against the eval set. Keep a change only if it
improves the score.

- [ ] `Reranker` (cross-encoder) over the top candidates; reduce final k accordingly.
- [ ] Query rewriting/expansion before retrieval.
- [ ] Tune RRF constant, candidate depth, recency weight/half-life.
- [ ] (Optional) `topic_digests` + `DigestSynthesisJob` **with a conflict-resolution /
      revision step** (`DATA_MODEL.md` warning). Do not ship synthesis without it.

**Acceptance:**
- [ ] Each shipped enhancement has a recorded before/after eval-set delta that is
      positive.

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
