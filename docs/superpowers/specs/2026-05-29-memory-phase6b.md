# Phase 6b — Conversational Memory: consolidation, dual-circuit recall, synthesis eval

> **Implementation spec for a fresh Claude Code session.** Continues from
> `2026-05-29-memory-phase6a.md`. 6a delivered per-turn capture, subject-isolated recall,
> reinforce-on-recall, and read-side selection. 6b adds the **synthesis layer** and turns on
> the pieces 6a deliberately deferred to protect the eval baseline.
>
> Synthesis is the lossy, hard-to-trace part of the system (SPEC-GC §9 calls LLM
> consolidation a last resort). **Do not ship it without the revision step and the eval
> set below.**

## What 6a left in place for 6b

- `project_id` column is populated on every project row but **not yet filtered** on recall.
- Recall ranks project rows by relevance + a project-arm recency tiebreak only; the **full
  decay blend is off.**
- `ContextBuilder` selection (MMR + budget) is live (or a flagged pass-through to finish here).
- No synthesis rows exist; recall is single-circuit (atomic entries only).

## Components

### 1. `Consolidator` (`consolidator.go`) — periodic synthesis with mandatory revision

Batch job, **not** per-turn. Synthesizes related atomic project entries for a subject into a
durable topic summary, so procedural questions ("how did I implement the GC") resolve to one
coherent answer instead of scattered fragments.

- **Trigger (pre-decided):** turn-count buffer (default 20) **plus** a periodic timer
  fallback, per subject. Both config.
- **Mandatory revision / conflict-resolution step (non-negotiable):** the
  `DigestSynthesisJob` warning in `DATA_MODEL.md` applies — the StructMem idea has *no*
  conflict resolution and drifts. Each synthesis run must reconcile the new summary against
  the previous summary **and** against `superseded_by` on its source entries, retiring stale
  claims rather than accumulating them. A contradiction is resolved via the GC's `Supersede()`,
  never by silently overwriting.
- **Storage:** synthesis summaries are `chunks` rows (reuse the table), distinguished by a
  marker in `metadata` (e.g. `kind: "synthesis"`) and carrying `subject_id` + `project_id` +
  `source_ids`. They are embedded and BM25-indexed like any row, so recall finds them
  natively. (A separate `topic_digests` table is optional and only if querying summaries
  independently becomes necessary — start single-table per SPEC-GC §9.)
- Uses the `BRAIN_*` LLM via the same client as the Extractor; prompt versioned + `go:embed`ed
  (`prompts/consolidate_v1.txt`), version stamped into `metadata`.

### 2. Dual-circuit recall

Both retrieval arms already search all of the subject's project rows; synthesis rows are now
among them. Make recall **prefer a matching synthesis summary for procedural queries** and use
atomic entries to fill detail:

- A summary that matches is the pre-compressed "how" — surface it first, then let MMR add
  distinct supporting atoms within the token budget.
- This is what makes "how did I implement X" cheap and coherent. Without consolidation, recall
  returns shrapnel.

Keep it behind the existing `Retriever` / `ContextBuilder` interfaces — no caller change.

### 3. `project_id` filtering — the "current project" use case

`Query.ProjectID` is now honored: when set, scope recall to that project's rows (plus the
global KB). Serves *"the recent feature of the current project I'm working on."* Population
happened in 6a; this is the filter clause + the resolution of *which* project is "current"
(from session metadata — see open questions).

### 4. Re-enable the full decay-rank blend for project rows

Turn on SPEC-GC §4's decay blend so stale project memories sink in ranking *before* the sweep
archives them. Global rows keep full rank (schema-gated). Because this changes ranking, it
must be **measured against the eval set** (below) before it is trusted — run before/after and
keep only if recall holds or improves.

### 5. Synonym / alias expansion (recall quality)

BM25 treats "GC" and "garbage collector" as unrelated tokens. Add light query expansion
(append known aliases before the BM25 query) or a small in-repo alias map. Cheap, and it
closes the most common keyword blind spot for exactly these use cases. Measure the delta.

### 6. Optional robustness (only if measured as needed)

- Durable capture outbox (replaces 6a's drop-on-restart) if dropped captures prove costly.
- Inline extract retry with backoff.
- Richer importance scoring beyond the 6a fact-vs-relational heuristic.

## Build checklist

- [ ] `prompts/consolidate_v1.txt` (`go:embed`) + `consolidator.go`: buffered/timed trigger,
      synthesis, **revision step** reconciling against prior summary + `superseded_by`,
      `Supersede()` for contradictions. Synthesis rows tagged `kind:"synthesis"` with
      `source_ids`.
- [ ] Dual-circuit recall: prefer matching synthesis summary, fill with atoms via MMR.
- [ ] Honor `Query.ProjectID` filter; resolve "current project" from session metadata.
- [ ] Re-enable decay blend for project rows (global untouched); measure on the eval set.
- [ ] Alias/synonym expansion; measure on the eval set.
- [ ] Multi-session synthesis eval set (below).

## Acceptance criteria

- [ ] Synthesis runs on a timer/threshold with a revision step that resolves contradictions
      (test: feed contradicting facts across sessions → the summary reflects current truth,
      the stale claim is superseded, not duplicated).
- [ ] "How did I implement X" returns a coherent consolidated answer, not scattered fragments
      (dual-circuit test).
- [ ] A `project_id`-scoped query returns only that project's rows + global KB.
- [ ] Decay blend re-enabled: recall on the eval set holds or improves vs the pre-blend score
      (recorded before/after).
- [ ] Alias expansion: a "GC" query retrieves a "garbage collector" entry it previously missed,
      with a recorded eval delta.
- [ ] Synthesis is **not merged** until the synthesis eval set passes.

## Multi-session synthesis eval set (build here, use forever)

- [ ] 20–40 multi-session scenarios with known-correct consolidated answers and/or expected
      source entry ids: multi-turn build-up of a topic across sessions, a mid-stream
      correction, then a procedural query.
- [ ] Runner reports recall@k and consolidated-answer correctness (LLM-as-judge or manual).
- [ ] Run at the end of 6b and before/after every ranking change (decay blend, aliases, MMR λ).

## Tuning knobs — flag to the user, do not hard-bake

All placeholders, to be tuned against **real chat volume** using `Stats()` before being
trusted: consolidation buffer (20) and timer; MMR λ (0.7) and final k (5–8); candidate depth
(20–40); decay `lambda` (0.05), archive threshold (0.1), sweep interval (1h); trigram
threshold (0.7); alias map contents.

→ Phase 6 complete when 6a + 6b acceptance criteria pass and the synthesis eval set is green.

---

## Implementation decisions (confirmed 2026-05-29, before planning)

These resolve the spec's open questions for the 6b build, decided with the user:

1. **Synthesis eval scope:** build the full eval **runner** (recall@k + consolidated-answer judging)
   plus an initial **~12 seed scenarios** that gate the Consolidator merge now, structured so more
   can be appended toward the spec's 20–40 target. The shortfall vs 20–40 is flagged, not hidden.
2. **Eval DB:** use a **dedicated eval database** (`MEMORY_EVAL_DATABASE_URL`, falling back to
   `DATABASE_URL`) so the unit-test suite (which recreates `chunks` at `vector(4)` via `freshDB`)
   and the eval/synthesis harness (which needs `vector(768)`) stop clobbering each other's
   embedding dimension. Ends the recurring dim conflict; unit tests are unchanged.
3. **`project_id` filtering:** honor an explicitly-set `Query.ProjectID` (scope recall to that
   project's rows + the global KB). **Defer** "resolve which project is current from session
   metadata" to whoever wires the agent loop — nothing consumes session metadata yet (the 6a
   capture hook is still unwired). Same deferral pattern 6a used for capture wiring.

**Baseline protection (carried from 6a):** the all-global eval builds `Query{Text,K}` with nil
`SubjectID`/`ProjectID`. Every recall change in 6b must be eval-neutral by construction — the
project filter is gated on `$projectID IS NULL` (no-op when unset), and the decay blend multiplies
**only** project-row scores (`scope='global'` rows are multiplied by exactly `1.0` → byte-identical).
Alias expansion is the one eval-**affecting** change: it ships config-gated (default off), and its
recall delta is measured on the eval set before being trusted.
