# Phase 5 — Memory GC (supersession + dedup)

> **Bridge document.** Continues from `PHASE_4_MEMORY_GC_CORE.md`. **The Memory-GC package's
> `SPEC.md` §5 is the source of truth** for the duplicate-vs-correction logic. Corresponds to
> the GC package's own internal **Phases 4–5** (supersession → dedup).

## Placement & dependencies

- **After Phase 4** (GC core). Adds capability; build when needed.
- Depends on `pg_trgm` (enabled in Phase 4; available on ParadeDB).

## Integration touch-points

1. **Supersession completes the freshness/news layer.** The GC's version chain
   (`supersedes` / `superseded_by` / `version`, history preserved, retrieval shows only
   `active`) is a **superset** of any simple Phase 2 `superseded_by` flag. Make it the single
   supersession path; if a simpler form was shipped earlier, migrate those rows onto the
   version-chain model — do not run two schemes in parallel.
2. **`confidence` flow = news/event verification.** First report stored `confidence='low'`;
   a verified correction supersedes it at `confidence='high'`, original kept and linked. This
   is where evolving/correctable facts live.
3. **Dedup is scope-gated and off the hot path.** Trigram runs in the sweep over
   recently-inserted rows, **within scope**. The safe fallback (SPEC §5: similar-but-ambiguous
   → **keep both**, let ranking prefer the newer) is mandatory. A contradiction must **never**
   be collapsed as a duplicate.
4. **No embeddings in the comparison path** (SPEC §9): dedup uses structured-fact lookup +
   trigram + "keep both" only. Rows still carry `embedding` for *retrieval* — not in conflict.

## Build (per the GC package's TASKS, internal Phases 4–5)

- [ ] `Supersede()` — transactional: insert new row with `supersedes` + `version+1`, mark old
      `superseded` with `superseded_by`, audit.
- [ ] `History()` — recursive version-chain query.
- [ ] Structured-fact path at ingest: `fact_subject`+`fact_predicate` set → same value =
      reinforce (duplicate); different value = `Supersede()`.
- [ ] Wire `confidence` for the news/verification flow.
- [ ] `dedupRecent()` in the sweep: trigram within scope, threshold from config; clear
      duplicate → reinforce survivor + mark loser `dead 'duplicate'`; ambiguous → keep both.
- [ ] Sweep pass order: dedup → supersession → archive → purge.

## Acceptance criteria

- [ ] Superseding hides the old row from retrieval, shows the new one, keeps the chain
      walkable, and is fully reversible.
- [ ] A contradiction pair is **never** collapsed — a test inserts one and asserts both
      survive (SPEC §5 non-negotiable).
- [ ] Near-identical duplicates collapse to one reinforced survivor.
- [ ] One unified supersession path (no leftover parallel scheme).
- [ ] Dedup redundancy is **measured before the pass is enabled** (run the eval set
      before/after so collection can't silently cut recall).
- [ ] Trigram similarity threshold is config.

→ **Next: Phase 6 — conversational memory now consumes `Supersede()` / `Reinforce()`.**
