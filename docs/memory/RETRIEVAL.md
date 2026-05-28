# Retrieval

This is the heart of the module. The goal: for a query, return the most relevant **and
current** chunks, combining semantic (vector) and keyword (BM25) signals.

---

## Fusion: Reciprocal Rank Fusion (RRF)

Vector distance and BM25 scores are on different, non-comparable scales. RRF sidesteps
this by combining **rank positions** instead of raw scores:

```
rrf_score(chunk) = Σ over each retriever of  1 / (C + rank_in_that_list)
```

`C` is a constant (use **60**). A chunk appearing in only one list still scores from that
list. No normalization, no tuning to start. This is the default; do not reach for
weighted schemes until the eval set shows RRF is the bottleneck.

---

## BM25 backend options (resolve during prerequisites)

The query below uses **`pg_textsearch`** operators. Pick the backend your host supports
and adjust operators/index accordingly — all behind the `Retriever` interface:

| Backend | Match operator | Notes |
|---------|----------------|-------|
| `pg_textsearch` (Timescale/Tiger) | `content <@> to_bm25query($1)` | Preferred; true BM25. |
| VectorChord-bm25 | its native bm25 operator | True BM25; different syntax. |
| ParadeDB `pg_search` | `@@@` | True BM25; Tantivy-backed. |
| Native `tsvector` (built-in) | `ts_rank(...)` | MVP only; weaker ranking, no extension needed. |

If you must fall back to `tsvector` for Phase 1, keep the `Bm25Retriever` interface
identical so upgrading later is a one-file change.

---

## The hybrid query (Phase 1 + Phase 2)

Single round trip: both retrieval arms as CTEs, ranked, then RRF — plus the freshness
filters and recency term. **Parameterize everything.**

```sql
-- Params: $1 user text · $2 query embedding · $3 candidate depth
--         $4 final k    · $5 as_of timestamp (default now())
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY content <@> to_bm25query($1)) AS rank
    FROM chunks
    WHERE superseded_by IS NULL              -- Phase 2: live only
      AND published_at <= $5                 -- Phase 2: point-in-time
    ORDER BY content <@> to_bm25query($1)
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
SELECT
    c.id,
    c.content,
    COALESCE(1.0 / (60 + b.rank), 0)
  + COALESCE(1.0 / (60 + v.rank), 0)
  + 0.1 * exp(-extract(epoch FROM now() - c.published_at) / 2592000)  -- Phase 2: recency
        AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE b.id IS NOT NULL OR v.id IS NOT NULL
ORDER BY score DESC
LIMIT $4;
```

### How this maps to phases

- **Phase 0** (vector only): just the `vec` CTE, no RRF, no freshness clauses, no recency
  term. Order by `embedding <=> $2`.
- **Phase 1** (hybrid): add the `bm25` CTE and the RRF sum. No freshness yet.
- **Phase 2** (freshness): add the two `WHERE` clauses and the recency term.

The query **grows in place** — the same statement, extended. That is the payoff of the
phased design; do not rewrite it each phase.

### Notes

- `<=>` is pgvector cosine distance (smaller = closer). Match it to the index opclass in
  `DATA_MODEL.md` (`vector_cosine_ops`).
- **Phase 3:** the `0.1` recency weight and `2592000` half-life seconds are no
  longer SQL constants — they are bound as `$6` and `$7` from `MemoryConfig.RecencyWeight`
  and `MemoryConfig.RecencyHalfLife`, with env overrides `RAG_RECENCY_WEIGHT` and
  `RAG_RECENCY_HALFLIFE_DAYS`. Defaults preserve Phase 2 behavior. The `cmd/memory-eval
  -sweep` mode runs a 3x3 grid over these two knobs and prints the recommended values.
- Keep **candidate depth (`$3`) generous** (20–40) even though **final k (`$4`)** is
  small. The extra candidates are what a Phase 3 reranker consumes.
- If multi-tenant, add `AND tenant_id = $8` to **both** CTEs.

---

## Point-in-time queries

Pass `as_of` to answer "as the corpus stood on date X." Default it to `now()`. With
`valid_from`/`valid_to` populated you can additionally constrain
`($5 BETWEEN valid_from AND COALESCE(valid_to, 'infinity'))`.

---

## Supersession (Phase 2)

When ingestion detects that a new chunk replaces an old one (same canonical source/key,
newer `published_at`), `SupersessionJob` sets `superseded_by = <new_id>` on the old row.
The query above filters `superseded_by IS NULL`, so stale facts stop being retrieved
without being deleted. Detection strategy (source key match, similarity threshold, or
explicit document versioning) is an **open question** for the team — see
`IMPLEMENTATION_PLAN.md`.
