# Agentic RAG rung-1 — eval results

Per the rung-1 refactor brief §3.7 ("eval gate"). This is the on-record delta between
the legacy retrieval pipeline and the agent loop on the frozen fixture corpus
(`internal/memory/eval/testdata/corpus/`, fixture_version=1) and seed
(`internal/memory/eval/testdata/seed.json`, 30 questions across four types).

See [ARCHITECTURE.md §Agentic query path](ARCHITECTURE.md#agentic-query-path-rung-1-behind-agent_loop_enabled)
for the loop's design.

## TL;DR

| Metric | Baseline (`AGENT_LOOP_ENABLED=false`) | Agent loop (`AGENT_LOOP_ENABLED=true`) |
|---|---|---|
| Mean recall@5 (retrieval, 30q) | **1.000** | n/a — different harness |
| Per-type recall@5 | paraphrase 1.000, exact_token 1.000, fresh_vs_stale 1.000 | n/a |
| Give-up correctness (30q) | n/a — no give-up terminal | **25/30 (83.3%)** |
| Negative-type give-up (2q) | n/a | **2/2 (100%)** |
| Answer grounding (citation must reference accumulated chunk) | n/a — no grounding gate | **23/23 answered (100%)** |

The two harnesses are orthogonal:

- **Retrieval recall@5** (`make eval-memory`) scores whether the expected doc lands in the
  top-5 chunks. It never calls the generator, so the agent loop is invisible to it.
- **Agent eval** (`make eval-memory-agent`, `-agent` mode) runs each seed question through
  `AgentLoop.Run` and scores `give_up_correct` and `grounded`.

Both are run side-by-side here. The recall@5 line is the no-regression guard; the agent
eval is where the loop earns its place.

## Acceptance gate (§3.7) analysis

- **"Overall recall@5 must not regress by more than 1pp."** ✓ Held trivially — recall@5 is a
  retrieval-only metric and the agent loop reuses the same retriever. Baseline 1.000 is
  unchanged; the agent path does not touch the retrieval harness.
- **"For at least one question type (likely negative) agent loop should show measurable
  improvement, because `give_up` is now an available terminal."** ✓ Both negative seeds
  (`n01-no-grpc`, `n02-no-billing`) produced `give_up` with a coherent reason citing the
  retrieved chunks' irrelevance. The legacy pipeline has no give-up terminal — it would
  hand whatever the retriever surfaced to the generator and produce a hallucinated answer
  over weak context. The agent loop refuses cleanly. This is the spec's intended win.
- **Bonus: grounding gate.** 23/23 answered questions carried only citations that actually
  pointed at accumulated chunks. Zero ungrounded answers slipped through. The
  `isGrounded` check ([agent_loop.go:116-131](../../internal/memory/agent_loop.go#L116))
  catches body-level hallucinations the generator's structured-citation filter would miss.

**Gate verdict: PASS.** The feature flag stays default-off per the brief; the agent path
is wired and verified, and the flag can be flipped on a per-environment basis.

## What the agent gets wrong (5 false give-ups / 28 answerable)

These 5 answerable questions produced `give_up` when an answer was available in the
retrieved chunks:

| ID | Type | Question | Likely cause |
|---|---|---|---|
| `p08-purpose-paraphrase` | paraphrase | "what is the goal of the ptolemy project?" | Planner judged the retrieved purpose chunk insufficient |
| `e01-rrf-constant` | exact_token | `RRF C=60` | Very short query; planner couldn't infer intent |
| `e02-bm25-operator` | exact_token | `@@@` | Single-token operator query; same |
| `e05-superseded-by` | exact_token | `superseded-by` | Bare identifier; planner over-conservative |
| `e07-cosine-ops` | exact_token | `cosine ops` | Two-token query; same |

Pattern: 4 of 5 false give-ups are `exact_token` queries — very short, lexical lookups
where the planner has little context to decide. The retriever surfaces the right chunk
(recall@5 is 1.000 on these), but the planner doesn't trust it enough to commit to an
answer. **Out-of-scope here** — this is exactly the kind of signal that motivates the
follow-up rungs in §6 of the brief (query rewriting, `judge_sufficient`).

## Per-question agent loop verdicts

```
[ANSWER] p01-rrf-paraphrase  cites=[1acd666aa1e79f96#0 c28e7ff95b190345#2]
[ANSWER] p02-bm25-paraphrase  cites=[c28e7ff95b190345#0 c28e7ff95b190345#1]
[ANSWER] p03-fileops-paraphrase  cites=[46e2974d1c369b9e#1 46e2974d1c369b9e#3 46e2974d1c369b9e#4]
[ANSWER] p04-denypolicy-paraphrase  cites=[0ebe1beda4a86886#2 0ebe1beda4a86886#1]
[ANSWER] p05-supersession-paraphrase  cites=[8936138949c2f383#4 8936138949c2f383#0 8936138949c2f383#3]
[ANSWER] p06-hnsw-paraphrase  cites=[e8d78b135b17d4de#0 e8d78b135b17d4de#1]
[ANSWER] p07-recency-paraphrase  cites=[8936138949c2f383#3 8936138949c2f383#2]
[GIVEUP] p08-purpose-paraphrase  cites=[]
[ANSWER] p09-rrf-paraphrase-2  cites=[1acd666aa1e79f96#0 1acd666aa1e79f96#1 1acd666aa1e79f96#2]
[ANSWER] p10-supersession-paraphrase-2  cites=[8936138949c2f383#0]
[ANSWER] p11-hnsw-paraphrase-2  cites=[e8d78b135b17d4de#3 e8d78b135b17d4de#2]
[ANSWER] p12-policy-paraphrase  cites=[207c5ab089b42b80#4]
[GIVEUP] e01-rrf-constant  cites=[]
[GIVEUP] e02-bm25-operator  cites=[]
[ANSWER] e03-guarded-fileops  cites=[46e2974d1c369b9e#0 46e2974d1c369b9e#1 0ebe1beda4a86886#4]
[ANSWER] e04-deny-policy-write  cites=[0ebe1beda4a86886#1 0ebe1beda4a86886#3 0ebe1beda4a86886#2 d2d0eb77c314b604#0]
[GIVEUP] e05-superseded-by  cites=[]
[ANSWER] e06-hnsw  cites=[e8d78b135b17d4de#4 e8d78b135b17d4de#3 e8d78b135b17d4de#1 6cc9118670b199da#0]
[GIVEUP] e07-cosine-ops  cites=[]
[ANSWER] e08-half-life  cites=[aa26298a8b9008e8#2 aa26298a8b9008e8#3 aa26298a8b9008e8#1]
[ANSWER] f01-policy-ttl-current  cites=[207c5ab089b42b80#3 d2d0eb77c314b604#2 207c5ab089b42b80#2 d2d0eb77c314b604#0]
[ANSWER] f02-policy-ttl-history  cites=[d2d0eb77c314b604#3]
[ANSWER] f03-topk-default-current  cites=[c097e2958e2b736d#0 c097e2958e2b736d#1]
[ANSWER] f04-topk-default-history  cites=[4c840ab166ace6a0#0 4c840ab166ace6a0#1]
[ANSWER] f05-policy-paraphrase-fresh  cites=[207c5ab089b42b80#2 c28e7ff95b190345#5 d2d0eb77c314b604#3 207c5ab089b42b80#3]
[ANSWER] f06-policy-paraphrase-stale  cites=[207c5ab089b42b80#2]
[ANSWER] f07-topk-paraphrase-fresh  cites=[c097e2958e2b736d#0]
[ANSWER] f08-topk-paraphrase-stale  cites=[c097e2958e2b736d#0 4c840ab166ace6a0#1]
[GIVEUP] n01-no-grpc  cites=[]
[GIVEUP] n02-no-billing  cites=[]

give_up_correct = 25/30   grounded = 23/23 answered
```

Negative-type give-up reasons (verbatim from `agent_give_up` log):

- `n01-no-grpc`: *"The retrieved chunks contain irrelevant information about file
  operations, scoring methods (RRF), and service wrappers, with no mention of gRPC
  reflection service exposure."*
- `n02-no-billing`: *"The retrieved chunks do not contain any information about the
  billing provider used by Ptolemy."*

## Reproduction

```bash
# Baseline — retrieval-only recall@5
make eval-memory                       # equivalent to:
RAG_FIXTURE_DIR=internal/memory/eval/testdata/corpus \
RAG_CHUNK_SIZE_TOKENS=20 RAG_CHUNK_OVERLAP_TOKENS=10 \
  ./bin/ptolemy memory eval -seed internal/memory/eval/testdata/seed.json

# Agent loop — give_up_correct + grounded
make eval-memory-agent                 # equivalent to:
RAG_FIXTURE_DIR=internal/memory/eval/testdata/corpus \
RAG_CHUNK_SIZE_TOKENS=20 RAG_CHUNK_OVERLAP_TOKENS=10 \
AGENT_LOOP_ENABLED=true \
  ./bin/ptolemy memory eval -seed internal/memory/eval/testdata/seed.json -agent
```

Both runs require `.env` to provide `BRAIN_*`, `EMBEDDING_*`, and `DATABASE_URL` (or
`MEMORY_EVAL_DATABASE_URL`). `AGENT_MAX_STEPS` defaults to 5 in the eval target.

## Caveats

- Fixture corpus is 11 markdown docs (~2 KB each); seed is 30 questions. Results may
  shift on a larger, noisier corpus. Treat these numbers as a floor, not a ceiling.
- The agent eval is non-deterministic at the planner step (LLM output, even
  grammar-constrained, can vary across runs). Single-run numbers; if a regression is
  suspected, re-run before concluding.
- `negative` type is not reported as a separate row in the retrieval recall@5 summary
  because empty `expected_doc_ids` is treated as trivially "hit" by an empty retrieval
  set — the value of give-up is only observable in the agent harness.

## Follow-ups (out of scope for rung 1)

Per brief §6, the next rungs are gated on these eval results:

- **Query rewriting** — would target the 4 false give-ups on `exact_token` queries.
- **Iterative `judge_sufficient`** — would let the planner re-retrieve before giving up.
- **Memory-vs-docs routing** — after Phase 6 conversational memory lands.
- **External tool use** — only with a concrete use case.

Until those rungs ship and earn their own eval delta, the rung-1 loop stays as-is
behind the default-off flag.
