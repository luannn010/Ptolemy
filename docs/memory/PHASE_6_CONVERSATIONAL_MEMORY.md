# Phase 6 — Conversational Memory (capture & recall)

> The conversational layer, built **after** the Memory GC (Phases 4–5). Because the GC
> already owns lifecycle, this phase is **pure capture + recall that consumes the GC's
> `Reinforce()` and `Supersede()`** — it implements no decay, archiving, retention, or
> versioning of its own.

## Placement & dependencies

- **After Phases 4–5** (GC core + supersession/dedup).
- Depends on: the store + hybrid retrieval, and the GC's lifecycle methods (`Reinforce`,
  `Supersede`) and the `scope`-tagged schema — all in place by Phase 5.

## What this phase does — and does not

- **Does:** `MemoryGate`, `Extractor`, `PerTurnCaptureHook`, recall, `Consolidator`.
- **Does NOT:** implement decay, archive, purge, retention, `Reinforce`, or `Supersede` —
  those exist in the GC. Recall **calls** `Reinforce()`; a changed fact **calls**
  `Supersede()`.

## Pre-decided (do NOT re-ask)

1. **Subject = user (global), with a session tag.** `subject_id` = user id; entries also
   carry `session_id`. Isolation per user.
2. **Retention/lifecycle owned by the GC** (built in Phases 4–5). No TTL or cleanup here.
3. **Substrate = ParadeDB** — recall is hybrid.

## Components (Go, behind interfaces, repo idiom)

1. **`MemoryGate`** — cheap skip for trivial turns, no LLM call.
2. **`Extractor`** — turn → entries tagged `perspective` (`factual`|`relational`) + timestamp;
   grounded extraction. Uses the `BRAIN_*` LLM.
3. **`PerTurnCaptureHook`** — deterministic, **async/fire-and-forget**:
   `gate → extract → embed → INSERT` as `scope='project'`, `status='active'`, with
   `subject_id`, `session_id`, initial `importance`. (Columns already exist from Phase 4.)
4. **`Consolidator`** — periodic batch synthesis with a mandatory revision / conflict-
   resolution step. Uses the `BRAIN_*` LLM.
5. **Recall path** — hybrid retriever over this subject's project rows + a few synthesis
   summaries (dual-circuit), through `ContextBuilder`; then **call the GC's `Reinforce()`** on
   every returned entry.

## Data model

No new lifecycle schema — conversational entries are `scope='project'` rows in the GC-managed
table (columns from Phase 4). They set `subject_id`, `session_id`, `perspective`,
`importance`, `timestamp`, `embedding`, `content`. Index `embedding` (HNSW) and `content`
(BM25) so recall is hybrid.

## Design constraints

- Deterministic, **async** post-turn hook — not a model-decided tool.
- **Two-tier cadence**: cheap capture per turn; synthesis only periodic.
- **Tag for the GC**: `scope='project'` + `subject_id` + `session_id` + initial `importance`.
- **Recall reinforces** via the GC's `Reinforce()` — non-negotiable.
- **Supersession via the GC's `Supersede()`** — never an ad-hoc flag.
- **Defer all lifecycle to the GC** — no TTL, cleanup, or hard-delete here.
- Subject isolation on every recall; parameterized SQL; sensitive-data handling; prior tests
  stay green.

## Acceptance criteria

- [ ] An earlier-turn fact is recalled and used in a later turn/session.
- [ ] Trivial turns produce no entries (gate works).
- [ ] Capture is async — latency matches the no-capture baseline.
- [ ] Synthesis runs on a timer/threshold with a revision step resolving contradictions.
- [ ] A changed fact uses the GC's version-chain `Supersede()`; recall returns the new entry.
- [ ] Subject isolation: A never sees B's memory (test asserts).
- [ ] Recall calls `Reinforce()` on every returned entry (test asserts `access_count` bumped).
- [ ] Project entries are decay-eligible: the GC sweep archives an old, unaccessed one while
      a pinned/high-importance one survives.
- [ ] A multi-session memory eval set is created and scored; synthesis isn't trusted until it
      passes.

## Open questions — surface these

1. **Consolidation trigger**: time window, turn count, or buffer size?
2. **Same-turn recall**: must a fact captured this turn be usable next turn before
   consolidation? (Live context vs. stored.)
3. **Extraction prompt ownership & fidelity checks.**
4. **Initial `importance` heuristic** at capture (flat vs. signal-based) — sets the GC's decay
   ordering.
