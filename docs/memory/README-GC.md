# Memory Garbage Collector — Implementation Handoff

> **For a fresh Claude Code session.** This package is a complete spec + scaffolding for a
> memory garbage-collection subsystem. Read `docs/SPEC.md` first, then implement the stubs
> in `internal/memory/` following `docs/TASKS.md`. The schema is in `migrations/`.

## What this is

A memory store for an app that keeps **LLM chat conversations** alongside **research/reference
files** (PDFs, Word docs, tabular data). Stack: **Go + PostgreSQL + `pg_textsearch` (BM25-style
ranking via `ts_rank_cd`)**. The GC trims irrelevant/redundant memory **without** deleting
anything important, and without slowing the ingest or read paths.

## The one-paragraph mental model

Every memory has a **scope** (`project` = conversational, decays; `global` = reference, never
decays) and a **status** (`active` → `archived`/`superseded` → `dead`). Retrieval only ever sees
`active` rows. A memory's decay **score** is computed lazily from recency + access-count +
importance, gated so global/pinned rows are immune. All heavy work (de-duplication, supersession,
archiving) runs in **one background sweep** (`time.Ticker`), so ingest is a plain INSERT and reads
are a plain ranked SELECT. Nothing is ever hard-deleted on the live path — the GC flips a status
flag, and only long-`dead` rows are eventually purged.

## The five principles (do not violate)

1. **Never delete on the live path.** GC flips `status`; real purge only hits rows `dead` for 30d+.
2. **Scope is the decay boundary.** Decay queries are gated `WHERE scope = 'project'`. Global is protected by the *schema*, not a tunable.
3. **Duplicates collapse, contradictions version.** Same fact restated → reinforce one survivor. Fact changed/corrected → keep both, link them, prefer the new one. NEVER treat a contradiction as a duplicate.
4. **Reads reinforce.** Every retrieval bumps `access_count` + `last_accessed_at`.
5. **Hot path stays at DB speed.** No comparison on ingest, no external calls (no LLM, no embeddings). All comparison lives in the sweep.

## Files in this package

| File | Purpose |
|------|---------|
| `docs/SPEC.md` | Full technical specification. The source of truth. |
| `docs/TASKS.md` | Ordered, checkbox build plan. Implement in this order. |
| `migrations/0001_init.sql` | The complete schema + indexes + audit table. |
| `internal/memory/types.go` | The `Memory` struct + status/scope constants. |
| `internal/memory/score.go` | `decayScore()` — implemented (reference logic). |
| `internal/memory/store.go` | Ingest, retrieve, reinforce — **stubs with TODOs**. |
| `internal/memory/gc.go` | The sweep: ticker + 4 passes — **stubs with TODOs**. |
| `internal/memory/score_test.go` | Tests for the decay logic — implemented, must pass. |

## How to proceed (instructions for Claude Code)

1. Read `docs/SPEC.md` end to end before writing code.
2. Apply `migrations/0001_init.sql` (assume a `DATABASE_URL` env var; use `pgx`/`pgxpool`).
3. Work through `docs/TASKS.md` phase by phase. Each phase is independently testable.
4. Keep `score_test.go` green; add tests for `store.go` and `gc.go` as you implement them.
5. **Confirm before any destructive change.** The purge pass actually deletes — gate it behind
   a config flag (`GC_PURGE_ENABLED`, default false) and a 30-day floor.

## Tuning knobs (placeholders — flag these to the user, don't hard-bake)

- `lambda` (decay rate) — start `0.05`; bigger = forgets faster.
- archive threshold — start `0.1`; score below this archives a project row.
- trigram similarity threshold — start `0.7`; above this is a dedup candidate.
- sweep interval — start `1h`.
- purge grace period — `30 days` in `dead`.

These are guesses. They MUST be tuned against real data. Expose them as config, never as magic numbers buried in queries.
