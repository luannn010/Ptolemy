# Phase 4 — Memory GC (core lifecycle)

> **Bridge document.** Places the uploaded **Memory-GC package** (`README.md` / `SPEC.md` /
> `TASKS.md` + `internal/memory/` stubs) into the Ptolemy roadmap. **The GC package is the
> source of truth for the GC's internals**; this file defines only placement, dependencies,
> and integration. Corresponds to the GC package's own internal **Phases 0–3** (schema →
> scope gate → lazy decay → the sweep). You have a working, safe system at the end of it.

## Placement & dependencies

- **First of the memory-lifecycle phases — built BEFORE conversational memory (Phase 6).**
- Depends on **Phase 0** (the store, built) and hybrid retrieval (Phase 1 / available now on
  ParadeDB). It works on today's retriever and upgrades via the `Retriever` interface with no
  caller change.
- **Subsumes Phase 2 freshness:** the GC's supersession + `confidence` flow *is* the
  freshness/news mechanism for the document knowledge base (delivered fully in Phase 5).

## Why this comes first

- The unified, `scope`-tagged schema and the lifecycle machinery exist **before any
  conversational memory is written**, so those entries are GC-managed from first insert — no
  backfill later.
- **Supersession + dedup + audit immediately serve the `global` document KB.**
- The **decay/archive passes are built and tested now but sit dormant** — they target
  `scope='project'` rows, which don't exist until Phase 6.
- It shrinks Phase 6 to pure capture + recall that *consumes* this phase's `Reinforce()` /
  `Supersede()`.

## Foundational first step — unify the store

Extend the existing `chunks` table into the GC's `scope`-tagged table (do **not** create a
parallel `memories` table). Map: `scope='global'` = the document KB (existing `chunks`);
`scope='project'` = conversational memory (Phase 6). Add lifecycle columns
(`scope, status, importance, pinned, access_count, last_accessed_at,
supersedes/superseded_by/version, confidence`) and the `*_audit` table; keep `embedding`.
**This is migration #1 — everything downstream uses it.**

## Retrieval — keep hybrid, blend decay (ParadeDB)

The GC package's `Retrieve()` ranks with `ts_rank_cd`. **On ParadeDB, use its real BM25** for
the sparse arm, combine with the `pgvector` arm via RRF (per `docs/memory/RETRIEVAL.md`), and
multiply the `decay_score` into ranking for `scope='project'` rows only (global gets full
rank). Add the `status='active'` filter to both arms. Do **not** regress to lexical-only.

## Build (per the GC package's TASKS, internal Phases 0–3)

- [ ] Schema + audit: the unified table above; enable `pg_trgm`.
- [ ] Scope gate: `Ingest` (plain INSERT, tags `scope`/`status`), `Retrieve` (hybrid + decay
      blend + `status` filter), `Stats` (counts by scope×status).
- [ ] Lazy decay + reinforcement: `decayScore()`; `Reinforce()` (bump `access_count` +
      `last_accessed_at`) called from every read path.
- [ ] The sweep: ticker (config interval); `archiveDecayed()` with
      `WHERE scope='project' AND NOT pinned`, auditing each transition; gated `purgeDead()`
      (`GC_PURGE_ENABLED` default off, 30-day floor — the only destructive op).

## Non-interruption guarantee

The scope gate is a firewall: `global` immunity is enforced by the **schema, not a tunable**,
so the GC cannot decay/archive document-KB rows. All heavy work runs in the sweep; ingest
stays a plain INSERT and reads a ranked SELECT + cheap reinforcement UPDATE.

## Acceptance criteria

- [ ] A test inserts an old, unaccessed **synthetic** `scope='project'` row and asserts the
      sweep archives it, while a same-age `global` row is **untouched**.
- [ ] Hybrid retrieval preserved (vector arm present); decay multiplies project rows only.
- [ ] Every status transition is audited; archiving is reversible by one UPDATE.
- [ ] Ingest/read latency matches the pre-GC baseline; coverage gate held.
- [ ] `lambda`, archive threshold, sweep interval, purge grace, `GC_PURGE_ENABLED` are config.

→ **Next: Phase 5 — supersession + dedup.**
