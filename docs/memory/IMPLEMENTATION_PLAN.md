# Implementation Plan

Build in order. Each phase has tasks and **acceptance criteria** that must pass before the
next phase starts. Check boxes as you go.

> Before Phase 0: complete the **Prerequisites** checklist in `README.md` and detect the
> repo's language/framework. Set up one migration system and one config mechanism.

---

## Phase 0 — Vector MVP (close the loop)

Goal: an end-to-end pipeline you can query. Quality does not matter yet.

- [ ] Migration `0001_chunks_core` (table + `embedding` + HNSW index).
- [ ] `Loader` for the single most common source.
- [ ] `Parser` → clean text + metadata.
- [ ] `Chunker` — fixed-size + overlap, sizes in config.
- [ ] `Embedder` — batched calls to the embedding model.
- [ ] `Store.upsert` / `get` against Postgres.
- [ ] `VectorRetriever` — vector-only query (see `RETRIEVAL.md` Phase 0 form).
- [ ] `PassthroughFusion`.
- [ ] `ContextBuilder` — concatenate top-k with source ids, enforce a token budget.
- [ ] `Generator` — LLM call returning answer + citations.
- [ ] Orchestrator wiring components from config (no hard-coded retriever/fusion).

**Acceptance:**
- [ ] Ingesting a small known corpus then asking a question returns a grounded answer
      with at least one correct citation.
- [ ] Swapping the retriever implementation is a config change, not a code edit.

---

## Phase 1 — Hybrid (semantic + keyword)

Goal: add BM25 and fuse, for a big recall win on exact terms (codes, names, IDs).

- [ ] Confirm BM25 backend (`RETRIEVAL.md` → backend options); install extension.
- [ ] Migration `0002_chunks_bm25` (BM25 index).
- [ ] `Bm25Retriever`.
- [ ] `HybridRetriever` — single SQL query, both arms + RRF (Option A).
- [ ] `RrfFusion` (used now, or kept ready if you split to Option B later).
- [ ] Switch orchestrator config to the hybrid retriever.

**Acceptance:**
- [ ] A query containing an exact token (e.g. an error code or SKU) that vector-only
      missed in Phase 0 now returns the exact-match chunk.
- [ ] Paraphrase queries still work (semantic arm intact).
- [ ] Eval-set score (see below) is ≥ the Phase 0 score.

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

- [ ] Assemble **30–50 real questions** with known-correct answers and/or expected source
      chunk ids.
- [ ] A runner that executes the query path over the eval set and reports retrieval
      metrics (e.g. hit-rate / recall@k, and answer correctness via LLM-as-judge or manual
      labels).
- [ ] Run it at the end of every phase and before/after every Phase 3 change. Record
      scores in the repo.

---

## Open questions (need a human / repo decision — do not guess)

1. **Language & framework** of Ptolemy — implement in it.
2. **BM25 backend** — which extension the host supports (blocks Phase 1).
3. **Embedding model & dimension** — sets the schema and re-embedding cost.
4. **Multi-tenant?** — if yes, `tenant_id` filtering is mandatory on every query.
5. **Supersession detection strategy** — source-key match vs. similarity threshold vs.
   explicit document versioning. Affects `SupersessionJob` design.
6. **Is conversational/episodic memory needed?** — if Ptolemy must remember a user across
   sessions, that is a separate extension (StructMem-style) on this same store; flag it
   rather than bolting it on silently.

Surface these to the team before or during the phase that depends on them.
