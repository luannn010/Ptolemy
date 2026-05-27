# Ptolemy Memory Module — Implementation Spec

This folder is a build specification for the **memory / retrieval module** of Ptolemy.
It is written to be handed to **Claude Code** (or any engineer) and implemented phase by phase.

The design is a **hybrid Retrieval-Augmented Generation (RAG)** system on a single
PostgreSQL store, with a **freshness layer** for time-based content (news / updates /
a growing knowledge base). Every component sits behind a small interface so each phase
is an **add-on, not a rewrite**.

---

## How to use this spec (for Claude Code)

1. Read all five files before writing code. Order: `README` → `ARCHITECTURE` →
   `DATA_MODEL` → `RETRIEVAL` → `IMPLEMENTATION_PLAN`.
2. **Detect the repository's language and framework first** and implement everything in
   that stack. The interface signatures in this spec are language-neutral pseudocode —
   translate them, do not copy them verbatim.
3. Build **strictly in phase order** (0 → 1 → 2 → 3). Do not start a phase until the
   previous phase's acceptance criteria in `IMPLEMENTATION_PLAN.md` pass.
4. Ship the smallest working version of each component first, then enhance. Quality
   tuning belongs in Phase 3, not Phase 0.

---

## Prerequisites — verify BEFORE coding (hard blockers)

- [ ] **PostgreSQL** is available to Ptolemy and you can create extensions on it.
- [ ] **`pgvector`** extension can be installed (dense vector search).
- [ ] **`pg_textsearch`** extension can be installed for BM25. **This is host-dependent.**
      It is first-class on Tiger Data / Timescale and self-hostable, but many managed
      Postgres providers do not offer it. If it is unavailable, see
      `RETRIEVAL.md` → "BM25 backend options" for fallbacks (VectorChord-bm25,
      ParadeDB `pg_search`, or native `tsvector` for an MVP). The `Retriever` interface
      contains this choice, so the fallback does not change the rest of the system.
- [ ] An **embedding model** endpoint is available (e.g. a hosted embedding API or a
      local model). Record its **vector dimension** — the schema needs it.
- [ ] An **LLM** endpoint is available for generation.

If any blocker is unmet, stop and surface it rather than guessing.

---

## Scope and non-goals

**In scope:** ingesting documents, chunking, embedding, storing, hybrid (semantic +
keyword) retrieval, freshness/recency ranking, supersession (retiring stale facts),
point-in-time queries, building grounded prompts, and generating cited answers.

**Optional (Phase 3):** a reranker, query rewriting, and an "evolving topic digest"
synthesis layer (inspired by the StructMem paper).

**Explicit non-goals (unless the team decides otherwise later):**
- Full conversational/episodic agent memory (StructMem's core use case). This spec
  targets a **knowledge-base + freshness** memory. If Ptolemy later needs to remember a
  *user across sessions*, that is a separate extension built on the same store.
- Multi-modal (image/audio) retrieval.
- Real-time synchronous "write then immediately reason over it" guarantees. The heavy
  ingestion work is asynchronous and off the query path.

---

## Mini-glossary (shared vocabulary)

- **Embedding / vector** — a list of numbers representing a text's *meaning*; similar
  texts get similar vectors.
- **Dense retrieval** — search by meaning (vectors). Handles paraphrases.
- **Sparse / BM25 / lexical retrieval** — search by exact words. Handles codes, names,
  IDs, version numbers.
- **Hybrid retrieval** — run both and merge; covers each other's blind spots.
- **Chunk** — a small passage a document is split into; the unit of retrieval.
- **top-k** — return the k best matches.
- **RRF (Reciprocal Rank Fusion)** — merges two ranked lists using rank positions, not
  raw scores. No score normalization or tuning needed.
- **Reranker / cross-encoder** — a slower, more accurate second-pass scorer run on the
  top candidates only.
- **Grounding** — basing the answer on retrieved text rather than the model's memory;
  the main defense against hallucination.
- **Supersession** — marking an old chunk as replaced by a newer one.
- **Point-in-time / `as_of`** — "answer as the corpus stood on date X."
- **Eval set** — a fixed list of test questions with known-good answers, used to
  measure whether a change actually helped.

---

## File map

| File | Purpose |
|------|---------|
| `README.md` | This file — scope, prerequisites, glossary. |
| `ARCHITECTURE.md` | Components, their interfaces, and how they connect. |
| `DATA_MODEL.md` | PostgreSQL schema (tables, indexes, columns). |
| `RETRIEVAL.md` | The hybrid + recency + supersession query and fusion logic. |
| `IMPLEMENTATION_PLAN.md` | Ordered, checkboxed tasks and acceptance criteria per phase. |
