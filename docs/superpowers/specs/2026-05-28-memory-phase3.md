# Memory Module — Phase 3 (Eval-Set Hardening + Recency Tuning) Design

**Date:** 2026-05-28
**Branch base:** `main` at `06c4834` (after Phase 2 merge, PR #34).
**Work branch:** `ptolemy/memory-phase3`.
**Spec scope:** the first sub-PR of `docs/memory/IMPLEMENTATION_PLAN.md` §"Phase 3 — Enhancements". This PR delivers the measurement substrate (foundation) plus exactly one technical enhancement (recency tuning). Reranker, query expansion, and `topic_digests` are explicit deferrals to later Phase 3 sub-PRs.

## Goal

Phase 3 is the first phase whose spec acceptance is **"each shipped enhancement has a recorded before/after eval-set delta that is positive."** Acting on that demands a measurement surface that can detect deltas; today's eval (n=8, drifting with live docs) cannot. This PR delivers two coupled changes:

1. **Eval-set hardening** — pin the eval corpus to byte-stable fixtures, grow the question seed from 8 → ~30 with question-type tags, and teach the eval runner to report per-type recall@5. Without this, every later enhancement is unprovable.
2. **Recency tuning** — expose the previously hard-coded `0.1` recency weight and `2592000` half-life as configuration knobs (`RecencyWeight`, `RecencyHalfLife`), then run a 3×3 grid sweep against the hardened eval to either adopt new defaults or document a null result.

The two are bundled because recency tuning is *unmeasurable* without the fresh-vs-stale eval pairs introduced by the hardening; splitting them would land tuning code with no way to validate it.

The Phase 0/1/2 interfaces (`Retriever`, `Fusion`, `Store`, `Orchestrator`) are stable. Phase 3 plugs into them — the SQL query "grows in place" (`docs/memory/RETRIEVAL.md` pattern) by parameterizing two constants into `$6`/`$7`, and the eval harness adds new tagging without breaking its existing shape.

## Locked decisions (from brainstorming)

| Decision | Choice | Reasoning |
|---|---|---|
| Sub-PR scope | **Eval-set hardening + recency tuning only.** Reranker, query expansion, `topic_digests` deferred. | Spec says "add enhancements one at a time, measuring each." This PR ships the foundation + the technical enhancement whose lever is already in the code (`0.1` / `2592000` are spec-flagged tuning knobs). |
| Eval corpus location | **`internal/memory/eval/testdata/corpus/`** — frozen markdown fixtures committed alongside the code that loads them. | Go toolchain ignores `testdata/`; these are machine inputs, not human documentation; co-location with `LoadFixtureCorpus` keeps the loader's contract obvious. Phase 1→2 showed live docs drift recall by 12.5pp from a single edit; fixtures eliminate that. |
| Eval seed location | **`internal/memory/eval/testdata/seed.json`** (moved from `docs/memory/eval/seed.json`; old path deleted). | Same lifecycle argument: seed is executable input, not docs. One-line pointer added in `docs/memory/IMPLEMENTATION_PLAN.md` Phase 3 for discoverability. |
| Corpus origin | **Snapshot — not reference — of current Ptolemy docs.** Byte-stable copies, ~12 docs, committed. | Snapshot avoids the synthetic-eval trap (testing against an invented model of your docs). The detectable-drift mode is preferable to the no-tell synthetic mode. "Snapshot, not reference" comment lives at the top of `LoadFixtureCorpus`. |
| Fixture versioning | **`fixtureVersion` constant in `LoadFixtureCorpus`**, stamped into the sweep table's footer and recorded in the PR description. | When fixtures are eventually resynced to evolving real docs, the version bump makes "two sweep tables on the same fixture version" trivially verifiable. Prevents quiet apples-to-oranges drift across PRs. |
| Seed size | **~30 questions** (8 existing + ~22 new). | n=8 means a single-question swing moves recall@5 by 12.5pp — noise. n≈30 drops that to ~3.3pp, low enough to trust deltas. |
| Seed authoring | **LLM-generate-then-curate** via the brain LLM at `:1090`. | Faster than pure hand-authoring; quality is preserved by the curation rules below. |
| Curation rule | **Reject any question whose >50% of distinctive tokens overlap with its expected answer-chunk's distinctive text**, then **hand-rephrase survivors** away from source wording. | The actual circularity risk is leaking *answer-chunk* tokens (which inflates recall directly), not source-doc tokens generally. Hand-rephrase is the second line of defense. |
| Seed type mix | ~12 paraphrase / ~8 exact-token / ~8 fresh-vs-stale (4 pairs × 2) / ~2 negative (no-answer). | Mirrors current 4+4 mix scaled up + introduces the new categories the sweep needs. Negative questions probe false-positives. |
| Knob plumbing | **`RecencyWeight float64` + `RecencyHalfLife time.Duration` on `MemoryConfig`**, env vars `RAG_RECENCY_WEIGHT` + `RAG_RECENCY_HALFLIFE_DAYS`, threaded through `Orchestrator` → `HybridRetriever`. | Single plumbing pass, once-and-done; production behavior preserved when env unset (defaults 0.1 / 30d). |
| Knob validation | **`RecencyWeight ≥ 0`**, **`RecencyHalfLife ≥ 1h`**, validated at `MemoryConfig` load. | Floors a `0` halflife (divide-by-zero in SQL) and rules out meaningless sub-hour values; well below the sweep's 7d minimum so it's not a sweep blocker. |
| SQL parameterization | **`$6 * exp(-extract(epoch FROM $5 - c.published_at) / $7)`** replacing `0.1 * ... / 2592000`. | The minimum surgical change — query shape unchanged, just two more bind parameters. |
| Sweep grid | **3×3: weight ∈ {0.05, 0.1, 0.2} × halflife ∈ {7d, 30d, 90d}** (9 cells). | Wide enough to see direction (factor-of-2 spans); small enough to run fast. Includes current defaults as a cell so "do nothing" is in the table. |
| Sweep execution | **Ingest fixture corpus ONCE per sweep run; run 9 query batches** against the same ingested data. | Sweep varies only query-side params — re-ingesting nine times would introduce nuisance variance (HNSW build, transient state) unrelated to what's being measured. Also reduces DB-reset authorizations from 9 to 1. |
| Edge-optimum warning | **Sweep emits "warning: optimum on grid edge — extend in <direction>"** when the best cell is at a boundary. | Stops false victory at a grid corner; defers extended sweep to a follow-up sub-PR. |
| Decision rule | **Adopt new defaults only if fresh-vs-stale recall@5 improves by ≥ 1pp AND full-seed recall@5 regresses by ≤ 1pp AND the optimum is interior (not on grid edge).** Otherwise keep 0.1/30d and record the null result. | Asymmetric improvement floor prevents adopting 0.1pp "wins" that are noise; full-seed regression cap prevents trading global recall for a narrow win; interior requirement prevents declaring victory at a corner. |
| Null-result handling | **Eval-set hardening still ships;** recency knobs revert to (or stay at) 0.1/30d; PR description records the null sweep table. | Spec is explicit: "discard the change if no positive delta." Null result *is* positive information — defaults are near-optimal. |
| Out-of-harness LLM use | **Brain LLM is used offline for question drafting only**, never on the query path. | Same evidence that justifies deferring query expansion + reranker (LLM at `:1090` is 60–180s per call — too slow per query). This is called out in the PR description so the deferral is grounded. |
| Out of scope | Reranker, query expansion, `topic_digests` / `DigestSynthesisJob`, LLM-as-judge, CI-integrated eval. | Each is a separate Phase 3 sub-PR. |

## Architecture

Phase 3 is purely additive to the Phase 2 pipeline. Two surface changes (eval harness + `MemoryConfig`) and one in-place SQL change.

### Eval harness extensions

Three additions to `internal/memory/eval/`:

```go
// eval.go (additions)

const fixtureVersion = 1   // bump when fixtures are resynced to real docs

type QuestionType string

const (
    QuestionParaphrase   QuestionType = "paraphrase"
    QuestionExactToken   QuestionType = "exact_token"
    QuestionFreshVsStale QuestionType = "fresh_vs_stale"
    QuestionNegative     QuestionType = "negative"
)

type EvalQuestion struct {
    // existing fields...
    QuestionType QuestionType `json:"question_type"`
}

// LoadFixtureCorpus reads byte-stable markdown fixtures from dir as Chunks.
// SNAPSHOT, NOT REFERENCE: these files are frozen copies, not symlinks. Bump
// fixtureVersion whenever they are resynced to evolving real docs.
func LoadFixtureCorpus(dir string) ([]Chunk, error) { ... }

// Summarize returns per-type recall@k AND overall recall@k.
type Summary struct {
    Overall     float64
    PerType     map[QuestionType]float64
    NPerType    map[QuestionType]int
    FixtureVer  int
}
func Summarize(results []Result, k int) Summary { ... }
```

`LoadFixtureCorpus` parses each `.md` file in `dir`; the ID is `sha256(rel_path)[:16]` (stable across runs); `published_at` is read from a YAML frontmatter block (`---\npublished_at: 2024-01-15T00:00:00Z\n---`) so fresh-vs-stale fixture pairs can set the past date deterministically. If frontmatter is absent, `published_at` defaults to a deterministic constant (`fixtureBaseTime`) — never `time.Now()`, which would re-introduce nondeterminism.

### `MemoryConfig` gains two fields

```go
// config.go (additions)

type MemoryConfig struct {
    // existing fields...
    RecencyWeight   float64       // env: RAG_RECENCY_WEIGHT (default 0.1)
    RecencyHalfLife time.Duration // env: RAG_RECENCY_HALFLIFE_DAYS (default 30d)
}

func (c *MemoryConfig) validate() error {
    if c.RecencyWeight < 0 {
        return fmt.Errorf("RecencyWeight must be >= 0, got %v", c.RecencyWeight)
    }
    if c.RecencyHalfLife < time.Hour {
        return fmt.Errorf("RecencyHalfLife must be >= 1h, got %v", c.RecencyHalfLife)
    }
    return nil
}
```

The env-loader parses `RAG_RECENCY_HALFLIFE_DAYS` as a float (so `7`, `0.5`, etc. work) and multiplies by `24 * time.Hour`. Defaults are applied before validation; defaults are unchanged from Phase 2 so production behavior is preserved when env is unset.

### `Orchestrator` threads the two fields

`NewOrchestrator` accepts a `MemoryConfig` and passes the two fields to `NewHybridRetriever`. No change to the `Orchestrator.Answer` flow itself.

### `HybridRetriever.hybridRrfQuery` grows two parameters

Phase 2's 5-param query becomes 7-param:

```sql
-- Params: $1 user text · $2 query embedding · $3 candidate depth
--         $4 final k    · $5 as_of timestamp
--         $6 recency weight (float)  · $7 recency halflife seconds (float)   -- NEW in Phase 3
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
      AND published_at <= $5
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE superseded_by IS NULL
      AND published_at <= $5
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0)
     + COALESCE(1.0 / (60 + v.rank), 0)
     + $6 * exp(-extract(epoch FROM $5 - c.published_at) / $7)   -- CHANGED in Phase 3
        AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
  AND c.published_at <= $5
ORDER BY score DESC
LIMIT $4
```

`NewHybridRetriever` gains two constructor parameters; the binds happen at query time. The RRF constant `60` stays hard-coded for now — tuning it is a separate Phase 3 lever, deferred.

`Bm25Retriever`, `VectorRetriever`, and `RrfFusion` are not touched: BM25's standalone path has no recency term, and the fusion math is unchanged.

### `cmd/memory-eval/main.go` gains two flags + a sweep mode

- `-question-type <type>` — filters the seed to a single type before running (used internally by `-sweep` for the fresh-vs-stale subset; also usable manually).
- `-sweep` — runs the 3×3 grid: one ingest, nine query batches with per-iteration `RecencyWeight`/`RecencyHalfLife` injection. Emits one markdown table to stdout with columns `weight | halflife | fresh-vs-stale recall@5 | full-seed recall@5 | Δ_fs | Δ_full`. Footer line lists `fixture_version=N`. If the best (fs_recall, full_recall) cell satisfies the decision rule's interior + improvement floors, prints `WINNER: weight=X halflife=Y`; otherwise prints `NO WINNER — keep defaults` or `WARNING: optimum on grid edge — extend in <direction>`.

The sweep loop has an explicit comment: `// Between iterations, ONLY recency params change; everything else (ingested corpus, embedder, brain LLM, RRF constant) is held fixed to isolate the recency effect.`

### `Makefile`

```makefile
eval-memory: ## Run eval against the fixture corpus.
	@RAG_FIXTURE_DIR=internal/memory/eval/testdata/corpus \
	 EVAL_CHUNK_SIZE=20 \
	 ./bin/memory-eval

eval-memory-sweep: ## 3x3 recency-tuning sweep; ingest once, query nine times.
	@RAG_FIXTURE_DIR=internal/memory/eval/testdata/corpus \
	 EVAL_CHUNK_SIZE=20 \
	 ./bin/memory-eval -sweep
```

Both targets assume the same DB-reset prelude the user runs today (`PGPASSWORD=... psql ... -c 'DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE;'`) — documented in the PR description, not added to the Makefile (the reset requires authorization per the auto-mode classifier).

## Components touched (file-level summary)

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/eval/testdata/corpus/*.md` | Create (~12) | Frozen snapshot of representative Ptolemy docs. Each with `published_at:` frontmatter. Fresh-vs-stale pairs share a stem (e.g., `policy-v1.md` / `policy-v2.md`). |
| `internal/memory/eval/testdata/seed.json` | Create | ~30 tagged questions (12 paraphrase + 8 exact-token + 8 fresh-vs-stale + 2 negative). |
| `docs/memory/eval/seed.json` | Delete | Lifecycle belongs to `internal/`, not `docs/`. |
| `docs/memory/IMPLEMENTATION_PLAN.md` | Modify | Tick Phase 3 boxes with file/test pointers; one-line pointer to new seed location. |
| `internal/memory/eval/eval.go` | Modify | Add `QuestionType`, `fixtureVersion`, `LoadFixtureCorpus`, per-type `Summarize`. |
| `internal/memory/eval/eval_test.go` | Modify | Tag round-trip, per-type recall, fixture loader unit tests. |
| `internal/memory/config.go` | Modify | Add `RecencyWeight`, `RecencyHalfLife` fields + validator + env parsing. |
| `internal/memory/config_test.go` | Modify | Default + validator unit tests. |
| `internal/memory/orchestrator.go` | Modify | Thread the two fields into `NewHybridRetriever`. |
| `internal/memory/orchestrator_test.go` | Modify | Constructor-capture test asserting threading. |
| `internal/memory/hybrid_retriever.go` | Modify | `hybridRrfQuery` becomes 7-param; constructor takes weight + halflife. |
| `internal/memory/hybrid_retriever_test.go` | Modify | Integration test for `$6`/`$7` semantics; defaults preserve Phase 2 behavior. |
| `cmd/memory-eval/main.go` | Modify | `-question-type` flag, `-sweep` mode, markdown-table emitter, decision-rule classifier. |
| `cmd/memory-eval/main_test.go` | Modify (or create) | Sweep mode tests with a fake runner (deterministic per-cell recalls): all-nine, table format, edge-optimum, no-winner, ingests-once. |
| `Makefile` | Modify | `eval-memory` target rewired to fixture corpus; new `eval-memory-sweep`. |
| `.env.example` | Modify | Document `RAG_RECENCY_WEIGHT`, `RAG_RECENCY_HALFLIFE_DAYS`, `RAG_FIXTURE_DIR`. |
| `docs/memory/RETRIEVAL.md` | Modify | Note the `0.1` / `2592000` constants are now `$6` / `$7` (config-driven). |

Two files explicitly **not** touched:
- `internal/memory/types.go` — no new data types are needed.
- `internal/memory/rrf_fusion.go` — RRF constant tuning is a separate Phase 3 lever (deferred).

## Data flow

### Normal eval run (`make eval-memory`)

```
$ make eval-memory
   │
   ▼
DB reset (manual, authorized) → ApplyMigrations
   │
   ▼
memory-eval  (reads RAG_FIXTURE_DIR from env, matching Phase 1/2 pattern)
   ├─ LoadFixtureCorpus($RAG_FIXTURE_DIR) → []Chunk  (stable IDs, frontmatter published_at)
   ├─ Embedder.Embed(...) → vectors
   ├─ Store.Upsert(chunks)                               // ingest ONCE
   ├─ LoadSeed("seed.json") → []EvalQuestion  (~30 tagged)
   ├─ for q in seed:
   │     Retriever.Retrieve(q.Text, depth=20, k=5)
   │     hits = HitsExpected(q, results)
   ├─ Summarize(results, k=5) →
   │     Summary{Overall: 0.XX,
   │             PerType: {paraphrase: 0.XX, exact_token: 0.XX,
   │                       fresh_vs_stale: 0.XX, negative: 0.XX},
   │             FixtureVer: 1}
   ▼
prints per-type table + overall recall@5 + fixture_version
```

### Sweep run (`make eval-memory-sweep`)

```
$ make eval-memory-sweep
   │
   ▼
DB reset (manual, authorized) → ApplyMigrations
   │
   ▼
memory-eval -sweep   (reads RAG_FIXTURE_DIR from env)
   ├─ LoadFixtureCorpus + Embedder + Store.Upsert         // ONCE
   ├─ LoadSeed
   ├─ baseline = run with (weight=0.1, halflife=30d)
   ├─ for (w, h) in grid [(0.05,7d) (0.05,30d) ... (0.2,90d)]:    // 9 cells
   │     // Between iterations, ONLY recency params change.
   │     hybridRetriever.WithRecency(w, h)   // mutates the prepared binds
   │     run all ~30 queries
   │     record cell summary
   ├─ classify:
   │     if WINNER (fs-Δ ≥ +1pp ∧ full-Δ ≥ -1pp ∧ interior): print WINNER line
   │     elif optimum on edge:                            print WARNING line
   │     else:                                            print NO WINNER line
   ▼
prints 3x3 markdown table + footer (fixture_version=1) + verdict line
```

## Error handling

| Failure | Where | Behaviour |
|---|---|---|
| `RAG_RECENCY_WEIGHT` not a float | `MemoryConfig` env load | Wrapped parse error; fail-fast at construction. `Orchestrator` never instantiated. |
| `RAG_RECENCY_HALFLIFE_DAYS` ≤ 0 | `MemoryConfig.validate` | "RecencyHalfLife must be ≥ 1h" — same error path. |
| `RAG_FIXTURE_DIR` set to nonexistent path | `LoadFixtureCorpus` | Wrapped `os.ReadDir` error; eval binary exits non-zero with a clear message. |
| Frontmatter `published_at` malformed | `LoadFixtureCorpus` | Per-file error includes the file path; eval exits non-zero. (Fixtures are committed — a malformed one is a code bug, not runtime input.) |
| `seed.json` references a `question_type` value not in the enum | `LoadSeed` | `TestLoadSeed_UnknownTypeIsError` — returns an error naming the question id and bad type. |
| Sweep iteration: a single cell errors during `Retrieve` | `cmd/memory-eval -sweep` | Recorded as `cell error: <wrapped>` in the table; sweep continues; if ANY cell errored, the verdict line is `INCONCLUSIVE — re-run after fixing cell errors`. |
| Decision-rule tie (two cells equally satisfy interior + floors) | `cmd/memory-eval -sweep` | Pick the cell with higher `fs_recall`; if still tied, the cell closer to the centroid `(0.1, 30d)`. Deterministic tie-break. |
| Decision-rule edge optimum AND interior winner both present | Same | "Interior wins" — interior cells take precedence; the edge optimum is reported but not adopted. |
| Live DB has Phase 2 schema only (no new migration needed) | `ApplyMigrations` | This PR adds no migration. `chunks` table unchanged. |

## Testing strategy

TDD per CLAUDE.md and `superpowers:test-driven-development`. Each task lands red → green → commit. Six TDD phases, matching the brainstorm's ordering.

### Phase 3.0 — Eval harness foundation

**Unit (no DB):**

| Test | File | Asserts |
|---|---|---|
| `TestLoadSeed_TagsRoundTrip` | `eval_test.go` | Loading + re-serializing a tagged question preserves `question_type`. |
| `TestLoadSeed_UnknownTypeIsError` | `eval_test.go` | Unknown `question_type` returns an error naming the bad value. |
| `TestSummarize_PerTypeRecall` | `eval_test.go` | Given mixed-type results, `Summary.PerType` reports correct per-type recall@k; `Overall` matches global mean. |
| `TestSummarize_StampsFixtureVersion` | `eval_test.go` | `Summary.FixtureVer` equals the package constant `fixtureVersion`. |
| `TestLoadFixtureCorpus_ReadsMarkdown` | `eval_test.go` | Reads `.md` files from a temp dir; returns one `Chunk` per file with content body. |
| `TestLoadFixtureCorpus_DeterministicIDs` | `eval_test.go` | Calling twice on the same fixtures returns identical IDs (`sha256(rel_path)[:16]`). |
| `TestLoadFixtureCorpus_FrontmatterPublishedAt` | `eval_test.go` | Frontmatter `published_at` parses to RFC3339; absent frontmatter falls back to the deterministic constant. |

**Integration (CLI):**

| Test | File | Asserts |
|---|---|---|
| `Test_CLI_QuestionTypeFlag_Filters` | `cmd/memory-eval/main_test.go` | `-question-type=fresh_vs_stale` runs only those questions in a stubbed harness. |

### Phase 3.1 — Fixture corpus + grown seed (data-only)

No tests added here — this is a data commit. The Phase 3.0 tests gate format correctness; Phase 3.5 (live sweep) gates substantive correctness.

### Phase 3.2 — Config knob plumbing

**Unit (no DB):**

| Test | File | Asserts |
|---|---|---|
| `TestMemoryConfig_RecencyDefaults` | `config_test.go` | Unset env → `RecencyWeight==0.1`, `RecencyHalfLife==30 * 24 * time.Hour`. |
| `TestMemoryConfig_RecencyEnvParsed` | `config_test.go` | `RAG_RECENCY_WEIGHT=0.2`, `RAG_RECENCY_HALFLIFE_DAYS=7` → 0.2 and 7*24h respectively. |
| `TestMemoryConfig_RecencyRejectsNegativeWeight` | `config_test.go` | `RAG_RECENCY_WEIGHT=-0.1` → validate error. |
| `TestMemoryConfig_RecencyRejectsZeroHalfLife` | `config_test.go` | `RAG_RECENCY_HALFLIFE_DAYS=0` → validate error. |
| `TestMemoryConfig_RecencyRejectsHalfLifeBelow1Hour` | `config_test.go` | `RAG_RECENCY_HALFLIFE_DAYS=0.01` (~14 min) → validate error. |
| `TestOrchestrator_PassesRecencyConfigToHybridRetriever` | `orchestrator_test.go` | Constructor capture: `NewHybridRetriever` receives the same weight + halflife that were in `MemoryConfig`. |

### Phase 3.3 — HybridRetriever SQL parameterization

**Integration (Postgres required; skip cleanly without `DATABASE_URL`):**

| Test | File | Asserts |
|---|---|---|
| `TestHybridRetriever_RecencyParamsRespected` | `hybrid_retriever_test.go` | Two chunks identical except `published_at` (10d apart). Two retriever configs: (w=0.05, h=7d) and (w=0.2, h=90d). For each config, the score delta between the two chunks matches `w * (exp(-Δt_old/h) - exp(-Δt_new/h))` within 1e-9. |
| `TestHybridRetriever_DefaultsMatchPhase2` | `hybrid_retriever_test.go` | With defaults (0.1, 30d), the recency-term contribution for a chunk 1d old vs 60d old matches the values computed by the Phase 2 hard-coded formula within float tolerance. (Sanity check that defaults preserve behavior.) |
| Existing `TestHybridRetriever_RecencyTermPresent`, `TestHybridRetriever_PrefersFreshOverStale`, `TestHybridRetriever_PointInTime` | `hybrid_retriever_test.go` | Must stay green (defaults unchanged → behavior unchanged). |

### Phase 3.4 — Sweep mode + Makefile

**Unit (CLI; fake runner):**

The fake runner returns a deterministic `Summary` keyed by `(weight, halflife)` so the sweep harness can be exercised without a real DB or model.

| Test | File | Asserts |
|---|---|---|
| `Test_SweepMode_RunsAllNineCells` | `cmd/memory-eval/main_test.go` | All 9 (weight × halflife) combinations are executed; cell-summary array has length 9. |
| `Test_SweepMode_IngestsCorpusOnce` | `cmd/memory-eval/main_test.go` | The fake runner's ingest invocation count = 1; query batch count = 9. (Enforces the once-ingest-nine-query optimization.) |
| `Test_SweepMode_EmitsMarkdownTable` | `cmd/memory-eval/main_test.go` | Output table has columns `weight \| halflife \| fs_recall \| full_recall \| Δ_fs \| Δ_full`, in that order; footer line contains `fixture_version=1`. |
| `Test_SweepMode_DeclaresWinnerInteriorOnly` | `cmd/memory-eval/main_test.go` | Synthetic recall map where the best cell is interior (`(0.1, 30d)`) → emits `WINNER: weight=0.1 halflife=720h0m0s`. |
| `Test_SweepMode_FlagsEdgeOptimum` | `cmd/memory-eval/main_test.go` | Synthetic recall map where the best cell is at `(0.2, 90d)` → emits `WARNING: optimum on grid edge — extend in (weight↑, halflife↑)`; does NOT emit `WINNER`. |
| `Test_SweepMode_NoWinnerWhenNoCellImprovesByOnePp` | `cmd/memory-eval/main_test.go` | Synthetic recall map where all cells are within ±0.5pp of baseline → emits `NO WINNER — keep defaults`. (Backs decision-rule outcome (ii).) |
| `Test_SweepMode_NoWinnerWhenFullSeedRegressionTooLarge` | `cmd/memory-eval/main_test.go` | Synthetic recall map where one cell improves fs by 5pp but regresses full by 2pp → still `NO WINNER`. (Backs the asymmetric floor.) |
| `Test_SweepMode_TieBreakClosestToCentroid` | `cmd/memory-eval/main_test.go` | Synthetic map with two equally good interior cells → the one closer to `(0.1, 30d)` wins. |

### Phase 3.5 — Live sweep + decision

No new tests; this is a measurement step. Recorded in the PR description.

### Phase 3.6 — Docs

No new tests; doc updates only.

### End-to-end (live services; manual, run before final review)

- `make smoke-memory` still produces a grounded answer (regression check on the brain LLM path).
- `make eval-memory` against the fixture corpus produces the baseline table — recorded in the PR description.
- `make eval-memory-sweep` produces the 3×3 table + verdict — recorded in the PR description.

## Acceptance verification

| Spec acceptance | How verified |
|---|---|
| "Each shipped enhancement has a recorded before/after eval-set delta that is positive." | **Eval-set hardening:** the new tagged seed + fixture corpus is the substrate, so its "delta" is the broadened coverage itself. The PR description carries the baseline-on-new-seed table and is explicit that the **old n=8 / recall=0.875 number is NOT comparable** (different sample, different distribution, different corpus). **Recency tuning:** the 3×3 sweep table is the before/after evidence. If a winner is adopted, the PR description records the precise delta. If no winner, the null result is documented and recency knobs revert to (or stay at) 0.1/30d — per spec's "discard the change" rule. The null result is positive information: it tells us the current defaults are near-optimal on our eval surface. |
| Foundation invariant: existing Phase 0/1/2 tests stay green. | `make test` in CI; the constructor + SQL changes preserve defaults so behavior is unchanged when env is unset. |
| Spec discipline on deferral: state *why* reranker + query expansion are deferred. | PR description: "Recency tuning ships in this PR because it adds no per-query LLM calls. Query expansion and reranker stay deferred until cheaper inference is available — the brain LLM at `:1090` is 60–180s per call on CPU, which is fine for offline question drafting but not acceptable on the query path." |

## Out of scope (deferred to later Phase 3 sub-PRs or beyond)

- **Reranker (cross-encoder)** — needs a hosted reranker model and an in-query network call. Defer to a follow-up sub-PR when inference is cheaper.
- **Query rewriting/expansion** — needs a per-query brain-LLM call (60–180s on CPU). Same blocker as reranker.
- **RRF constant `60` tuning** — separate sweep on a separate knob; out of scope to keep this PR's blast radius small.
- **Candidate depth (`depth=20`) tuning** — same reasoning.
- **`topic_digests` + `DigestSynthesisJob`** — requires the conflict-resolution / revision step the spec mandates; that's a sub-project, not a sub-PR. Defer to Phase 4 candidate work.
- **LLM-as-judge** — recall@k is what gates retriever changes; semantic correctness scoring is a separate tooling project.
- **CI-integrated eval** (running `make eval-memory` on every PR) — useful ops project, separate from Phase 3 measurement.
- **Memory Garbage Collector (Phase 4–6 GC docs)** — those untracked drafts cover a different subsystem (status-based GC for chat memory). Not bundled here; revisited after Phase 3 ships.
- **CLI `--as-of` flag** — Phase 2 deferral still stands.

## References

- [docs/memory/IMPLEMENTATION_PLAN.md](../../memory/IMPLEMENTATION_PLAN.md) §"Phase 3 — Enhancements" — acceptance criteria.
- [docs/memory/RETRIEVAL.md](../../memory/RETRIEVAL.md) — the "query grows in place" SQL pattern + recency-knob commentary.
- [docs/memory/DATA_MODEL.md](../../memory/DATA_MODEL.md) §"topic_digests" — warning about conflict-resolution (why we're deferring synthesis).
- [docs/memory/ARCHITECTURE.md](../../memory/ARCHITECTURE.md) §"Reranker" + §"Async jobs" — the deferred Phase 3 components.
- [docs/superpowers/specs/2026-05-28-memory-phase2.md](2026-05-28-memory-phase2.md) — Phase 2 spec; defines the recency term this phase tunes.
- [docs/superpowers/plans/2026-05-28-memory-phase2.md](../plans/2026-05-28-memory-phase2.md) — Phase 2 plan; pattern for the Phase 3 implementation plan.
