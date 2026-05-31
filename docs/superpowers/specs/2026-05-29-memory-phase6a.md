# Phase 6a — Conversational Memory: capture + recall core

> **Implementation spec for a fresh Claude Code session.** Builds the per-turn capture and
> subject-isolated recall on top of the Memory GC (Phases 4–5). This phase implements **no**
> decay, archiving, retention, or supersession of its own — it *consumes* the GC's
> `Reinforce()` / `Supersede()`. Consolidation, synthesis, and the decay-rank blend are
> **6b** (see `2026-05-29-memory-phase6b.md`).
>
> Read `docs/memory/SPEC-GC.md` §2 (data model), §4 (decay), §5 (supersession ladder) and
> `PHASE_4_MEMORY_GC_CORE.md` before writing code.

## What already exists (grounding — do not rebuild)

- **LLM client:** a `/v1/chat/completions` HTTP client (the `OpenAIGenerator` pattern).
  Reuse it for the Extractor, pointed at `BRAIN_*`.
- **`Embedder.Embed`** — turns text into vectors.
- **`Store`** — has `Upsert`, `Get`, `Supersede`, `Reinforce`, `LookupFact`.
- **`Orchestrator.Answer`** already calls `Reinforce(ids)` after retrieval (Phase 4). So
  "recall reinforces" is **wiring, not new logic**.
- The table is **`chunks`** (extended in Phase 4 migration #1 with the scope/lifecycle
  columns), **not** a separate `memories` table.

Missing today: gate, hook, extractor, the subject filter on recall, and read-side context
selection. That is the scope of 6a.

## The one principle this phase encodes — where trimming lives

Each kind of trimming happens at the layer where it is both **cheap** and **correct**:

1. **Capture (async): clean only.** Turn → atomic, coreference-resolved, self-contained
   entries. Removes *noise*, never *signal*. Runs off the user-perceived latency path.
2. **GC sweep (off-path, reversible):** dedup near-identical rows; decay/archive stale ones.
   Owned by Phases 4–5. Redundancy- and time-based trimming live here.
3. **Recall (hot path, cheap): select.** Query-aware reduction to "just enough context"
   (MMR + token budget in `ContextBuilder`). Relevance trimming lives here, where the query
   exists and nothing is lost.

**Never make a relevance or importance decision that drops content at capture.** There is no
query to trim against, and a wrong guess is an irreversible silent forget — the cardinal
failure of a memory store, and a violation of the GC's never-delete-on-write rule.

## Pre-decided defaults (recorded as decisions — do not re-ask)

1. **Subject = user (global across sessions), tagged per session.** `subject_id` = user id;
   entries also carry `session_id`. Isolation per user.
2. **Retention/lifecycle owned by the GC.** No TTL or cleanup here.
3. **Substrate = ParadeDB.** Recall stays hybrid (BM25 `@@@` + pgvector, RRF).
4. **Same-turn recall:** store-backed recall is **next-turn**; the current turn is served
   from live context (capture is async, so the row may not be indexed yet).
5. **Extraction prompt:** in-repo, versioned, `go:embed`ed (`prompts/extract_v1.txt`), with a
   deterministic grounding check (below).
6. **A "turn" = the full user+assistant exchange,** not the user message alone (decision in
   §3 — load-bearing for the "how did I implement X" use case).

---

## 1. Migration 0006 — schema (get this right; it is expensive to reverse)

Add to `chunks` (lifecycle columns already exist from Phase 4):

```sql
ALTER TABLE chunks
  ADD COLUMN subject_id  TEXT,
  ADD COLUMN session_id  TEXT,
  ADD COLUMN project_id  TEXT,        -- the "current project" dimension; see note
  ADD COLUMN perspective TEXT;

-- perspective is constrained but nullable (global KB rows leave it NULL)
ALTER TABLE chunks
  ADD CONSTRAINT chunks_perspective_chk
  CHECK (perspective IS NULL OR perspective IN ('factual','relational'));

-- ISOLATION INVARIANT, enforced by the schema (not the query): a project row MUST be
-- owned by a subject. Without this, a capture bug that inserts subject_id = NULL produces
-- a project row that matches the `subject_id IS NULL` arm of recall and leaks to EVERY user.
ALTER TABLE chunks
  ADD CONSTRAINT chunks_project_owned_chk
  CHECK (scope = 'global' OR subject_id IS NOT NULL);

-- recall filter index (project rows only; global KB unaffected)
CREATE INDEX chunks_subject ON chunks (subject_id, project_id)
  WHERE subject_id IS NOT NULL;
```

**Why `project_id` now even though 6a doesn't filter on it:** `subject_id` is user-global, so
without a project/workspace tag, recall cannot scope to "the current project the user is
working on" (the first headline use case). The column is one line now and an accurate backfill
is impossible after rows exist. **Capture must populate it from day one;** 6b adds the filter.

`Chunk` struct gains `SubjectID`, `SessionID`, `ProjectID`, `Perspective` as `*string`
(nullable, like `FactSubject`). `Upsert`/`Get` scan them. Global KB rows keep them `NULL`.

Stamp the extractor prompt version into `metadata` (`extractor_version: "extract_v1"`) on
every captured row, so entries are auditable and selectively re-extractable when the prompt
changes.

## 2. `MemoryGate` (`gate.go`) — deterministic, no LLM

`Gate(turn) bool` — a pure, fully unit-testable skip for turns not worth capturing:

- below a minimum content length;
- pure greetings / acknowledgements (small stopword + pattern set).

Add a **secret/PII guard** here: if the turn matches obvious-secret patterns (API keys,
bearer tokens, private keys, long high-entropy strings), the gate either skips the turn or
flags it so the Extractor redacts. Conversational turns leak credentials, and capture embeds
into long-lived storage — do not persist obvious secrets. Keep the pattern set in-repo and
unit-tested.

Satisfies *"trivial turns produce no entries."*

## 3. `Extractor` (`extractor.go`) — `BRAIN_*` LLM, grounded, self-contained

`Extract(ctx, exchange) ([]ExtractedEntry, error)`. **Input is the exchange (user message +
assistant reply), not the user message alone** — the *how* of an implementation almost always
lives in the assistant's text, so user-only extraction would capture questions and never
answers. Attribute correctly (the user decided/asked; the assistant supplied the detail).

The prompt (in `prompts/extract_v1.txt`, `go:embed`ed) must require each entry to be:

- **atomic** — one fact per entry;
- **coreference-resolved / self-contained** — no dangling "it / that / this"; the entry must
  be meaningful with zero surrounding context (it will be read months later in isolation);
- **declarative** — conversational filler stripped;
- **typed** — `perspective` = `factual` | `relational`;
- **structured when possible** — set `fact_subject` + `fact_predicate` for durable facts, so
  capture can route through the existing Phase 5 ladder.

**Grounding / fidelity check (deterministic) — but reconciled with coreference resolution.**
Reject an entry whose key terms are not supported by the exchange. Two corrections to the
naive version so the check does not fight self-containment:

- Ground against **the whole exchange window** the extractor saw (and, if available, the
  prior turn), not the single user message — otherwise a correctly resolved subject (the
  exchange said "it", the entry says "the GC sweep") is wrongly rejected.
- Match on **normalized / stemmed token overlap above a threshold**, not exact substring —
  otherwise "implemented" vs "implementation", "GC" vs "garbage collector" trip it.
- Additionally **reject entries that still contain dangling pronouns** — a grounded but
  non-self-contained entry is useless at recall.

**Testability:** a fake LLM client returns canned JSON, so parsing + fidelity rejection +
dangling-pronoun rejection are unit-tested deterministically. The real `BRAIN_*` LLM runs only
in a smoke target.

## 4. `PerTurnCaptureHook` (`capture.go`) — async, deterministic, durable enough

`Enqueue(exchange)` returns immediately (buffered channel → background worker). The worker
runs `processTurn`: `gate → extract → embed → Upsert` as `scope='project'`,
`status='active'`, with `subject_id`, `session_id`, `project_id`, `perspective`, and an
initial `importance` (see §6).

- **`processTurn` is directly callable in tests** (deterministic with fakes).
- A separate test asserts **`Enqueue` does not block** → *"latency matches no-capture
  baseline."*
- **Bounded channel + explicit overflow policy.** "Doesn't block" means a full channel must
  **drop** (it must not grow unbounded). Make the policy explicit, and **emit a metric** on
  drops and on extract/embed failures — silent forgetting must be observable.
- **Failure handling:** on extract/embed error, log + count and drop that exchange (do not
  retry inline on the hot path; a retry/outbox is a 6b option). Document that in-flight
  exchanges are lost on process restart (acceptable for 6a; durable outbox deferred).
- **Supersession reuses Phase 5:** when an entry carries `fact_subject`+`fact_predicate`,
  capture routes through the existing structured-fact ladder
  (`LookupFact` → same value = `Reinforce` (duplicate); different value = `Supersede()`). So
  *"a changed fact uses the GC's `Supersede()`"* needs **no new supersession code.**

## 5. Recall — subject isolation + reinforce (mostly wiring) + recency tiebreak

- `Query` gains `SubjectID *string` (and, reserved for 6b, `ProjectID *string`).
- The bm25 and vector arms gain: `AND (subject_id IS NULL OR subject_id = $N)`. A subject
  sees the global KB (`subject_id IS NULL`) **plus** their own project rows, **never** another
  subject's. The schema `CHECK` from §1 backstops this against capture bugs.
- **`Reinforce` already fires** in `Answer` → *"access_count bumped"* test passes unchanged.
- **Eval-neutral:** the eval corpus is all-global with no subject, so the filter matches every
  global row → `make eval-memory` stays **1.000**.

**Serving "recent" without breaking the baseline.** 6a defers the full decay-rank blend, so
recalled project rows would otherwise be ordered by relevance only and *"recent feature"*
would not be honored. Add a **recency tiebreak applied to the project arm only** (a small
recency term, or `last_accessed_at DESC` secondary order, inside the project-scope branch of
the union). The global arm's rank expression is **byte-identical**, and project rows do not
exist in the all-global eval → the **1.000 baseline is untouched**. This is the minimal slice
of the decay blend; the full blend is 6b.

## 6. `ContextBuilder` selection — the "just enough context" piece (read-side, cheap)

This is the relevance-trimming layer, and it belongs **here, not at capture.** After retrieval
returns a generous candidate set (depth 20–40), before building the prompt:

- **MMR** (Maximal Marginal Relevance) selects the final k (5–8):
  `argmax_d [ λ·rel(d,q) − (1−λ)·max_{d'∈selected} sim(d,d') ]`, λ ≈ 0.7. This drops
  near-duplicate fragments and forces coverage of *distinct* points — the actual mechanism
  for "use enough context, not more."
- A **hard token budget** caps what reaches the model.

Cost is a few hundred similarity ops over already-fetched vectors (microseconds) — negligible
against generation. Keep it behind the existing `ContextBuilder` interface so nothing
downstream changes.

> If 6a scope must stay minimal, MMR may ship as a thin pass-through and land fully in 6b —
> but flag it explicitly as the read-side trim, do **not** push relevance trimming to capture.

## 7. Initial `importance` — cheap signal, not flat

Set at capture (not deferred to a richer scorer):

- base flat default, **but** entries that resolved to `fact_subject`+`fact_predicate`
  (durable decisions) start slightly higher than loose `relational` chatter.

This sets better decay ordering for the GC at near-zero cost. Richer scoring is 6b.

---

## Build checklist (ordered)

- [ ] `migrations/0006_*.sql`: the four columns, `chunks_perspective_chk`,
      `chunks_project_owned_chk`, `chunks_subject` index. Extend `Chunk` + `Upsert`/`Get` scan.
- [ ] `gate.go`: `Gate(turn) bool` + secret/PII guard. Pure, unit-tested.
- [ ] `prompts/extract_v1.txt` (`go:embed`) + `extractor.go`: exchange-scoped, self-contained,
      typed extraction with the reconciled grounding check. Fake-LLM unit tests + smoke target.
- [ ] `capture.go`: `Enqueue` (non-blocking, bounded, metered) + `processTurn`
      (`gate→extract→embed→Upsert`, structured-fact ladder for supersession). `processTurn`
      directly callable in tests.
- [ ] Recall: add `SubjectID` to `Query`; add the `(subject_id IS NULL OR subject_id = $N)`
      clause to both arms; add the project-arm-only recency tiebreak.
- [ ] `context_builder.go`: MMR + token budget (or flagged pass-through if deferred).
- [ ] Initial `importance` heuristic in `processTurn`.
- [ ] Minimal 6a eval (below) committed and runnable.

## Acceptance criteria

- [ ] An earlier-turn fact is recalled and used in a later turn/session.
- [ ] Trivial turns produce no entries (gate test).
- [ ] Obvious secrets in a turn are not persisted (gate/redaction test).
- [ ] Capture is async — `Enqueue` does not block; latency matches the no-capture baseline.
- [ ] Extracted entries are self-contained — a dangling-pronoun entry is rejected (extractor
      test); a correctly coreference-resolved entry is **not** wrongly rejected by the
      grounding check (the reconciliation test).
- [ ] A changed structured fact routes through the GC's version-chain `Supersede()`; recall
      returns the new entry. (Reuses Phase 5 — no new supersession code.)
- [ ] **Subject isolation:** A never sees B's memory (test asserts), and the schema `CHECK`
      rejects a `scope='project'` row with `subject_id = NULL`.
- [ ] Recall calls `Reinforce()` on every returned entry (`access_count` bumped).
- [ ] Project entries are decay-eligible: the existing Phase 4 sweep archives an old, unaccessed
      project row while a pinned/high-importance one survives.
- [ ] Existing all-global eval stays **1.000** (no regression on the document KB).

## Minimal 6a eval — test what 6a actually builds (do not defer all of it)

The all-global "1.000 stays 1.000" check proves **no regression** on the old KB; it exercises
**none** of the new capture/recall behavior. Pull a small slice forward:

- [ ] 5–10 hand-written multi-session cases: feed an exchange → assert the extracted entry is
      clean, self-contained, correctly typed; in a later "session" → query → assert it is
      recalled, and that subject B does not see it.

The **full multi-session synthesis eval set** (scoring the Consolidator) is 6b — synthesis is
not trusted until it passes.

## Scope boundaries — explicitly deferred to 6b

- `Consolidator` (periodic synthesis + mandatory revision / conflict-resolution).
- Dual-circuit recall (atomic entries **+** synthesis summaries; prefer a summary for
  "how did I do X" queries).
- Full decay-rank blend re-enabled for project rows (6a keeps the project-arm recency tiebreak
  only, to protect the baseline).
- `project_id` **filtering** on recall (column is populated in 6a; filter is 6b).
- Full MMR if shipped as pass-through in 6a; durable capture outbox; richer importance scoring.

→ **Next: `2026-05-29-memory-phase6b.md` — consolidation, dual-circuit recall, decay blend,
synthesis eval.**
