# Memory GC — Technical Specification

This is the source of truth. The HTML design note and the code stubs all derive from this.

## 1. Problem

The store holds two species of data that age in opposite directions:

- **Project / conversational** (chat logs with LLMs): time-ordered, value mostly recent, old
  throwaway exchanges genuinely stop mattering. **Should decay.**
- **Global / reference** (PDFs, Word docs, tabular research data): durable knowledge, age is
  irrelevant to value, accessed infrequently but still important. **Must NOT decay.**

A single global decay rule cannot serve both. The design splits them by a `scope` tag and gates
all decay logic on it.

## 2. Data model

One table, `memories`. Key fields:

- `scope` — `'project'` | `'global'`. The decay boundary.
- `status` — `'active'` | `'archived'` | `'superseded'` | `'dead'`. Retrieval sees only `active`.
- `importance` (REAL), `pinned` (BOOL) — pinned/global are decay-immune.
- `access_count` (INT), `last_accessed_at` (TIMESTAMPTZ) — reinforcement signals.
- `confidence` — `'low'|'normal'|'high'` — for news/event verification flows.
- `supersedes`, `superseded_by` (BIGINT FK), `version` (INT) — the supersession chain.
- `fact_subject`, `fact_predicate` (TEXT, nullable) — optional structured-fact form for
  zero-cost contradiction detection on durable facts.
- `textsearch` (tsvector, generated) — the FTS column for BM25-style ranking.

Plus a `memory_audit` table logging every status transition (`memory_id, old_status,
new_status, reason, created_at`).

See `migrations/0001_init.sql` for the exact DDL and indexes.

## 3. The status state machine

```
            decay (score < threshold)
   active ───────────────────────────► archived ──(30d+)──► dead ──► [purge]
     │                                     │
     │ corrected by vN+1                   │ rollback (one UPDATE)
     ▼                                     ▼
 superseded                             active
```

- Only `active` is visible to normal retrieval.
- `archived` and `superseded` are hidden but fully recoverable.
- `dead` is purge-eligible, but purge is destructive and gated (config flag + 30-day floor).
- Rollback from `archived`/`superseded` back to `active` is a single `UPDATE`.

## 4. Decay scoring

Computed **lazily** (on read / in the sweep), never stored as a column that goes stale:

```
score = importance * exp( -lambda * days_since_access / (1 + access_count) )
```

- `global` scope or `pinned = true` → score is constant `1.0` (never decays).
- Reinforcement: high `access_count` flattens the curve, so frequently-used memories persist.
- `lambda` is the single decay dial (start `0.05`).

Decay is also blended into retrieval ranking so stale memories sink *before* they're archived:

```sql
-- project rows: BM25 rank scaled by decay; global rows: full rank. Union, order, limit.
SELECT id, content, ts_rank_cd(textsearch, q) * decay_score AS rank
  FROM memories, plainto_tsquery('english', $1) q
 WHERE textsearch @@ q AND scope = 'project' AND status = 'active'
UNION ALL
SELECT id, content, ts_rank_cd(textsearch, q) AS rank
  FROM memories, plainto_tsquery('english', $1) q
 WHERE textsearch @@ q AND scope = 'global' AND status = 'active'
ORDER BY rank DESC LIMIT 20;
```

(In Go, you can compute `decay_score` per row after fetching raw inputs, or push the exp() into
SQL as shown in the archive query in §6. Both are fine; keep it consistent.)

## 5. Duplicates vs. corrections — the critical distinction

Two memories can be textually similar for OPPOSITE reasons. Getting this wrong makes the store
confidently incorrect. **Never silently treat a contradiction as a duplicate.**

| Case | Example | Action |
|------|---------|--------|
| Duplicate | "prefers %w wrapping" said 3× | Collapse → reinforce one survivor, no new row |
| Supersession | "deploy AWS" → "moved to GCP" | Version → keep both, link, retrieval prefers new |
| Distinct | unrelated facts | Just insert |

### Detection ladder (cheapest first, NO external calls)

1. **Structured facts (~0ms):** if `fact_subject` + `fact_predicate` are set, a single indexed
   lookup decides it. Same subject+predicate+value = duplicate; same subject+predicate, different
   value = supersession.
2. **`pg_trgm` lexical (~few ms, in-DB):** for raw text, one GIN-indexed `similarity()` query.
   Gate it on **same scope**. Above the threshold = candidate pair.
3. **Safe fallback:** if similarity is high but you can't structurally tell duplicate from
   contradiction, **keep both and let recency ranking prefer the newer one.** No data lost, no
   LLM, no latency. This is the default when uncertain.

```sql
-- pg_trgm near-duplicate lookup, within scope
SELECT id, content, similarity(content, $1) AS sim
  FROM memories
 WHERE content % $1 AND scope = $2 AND status = 'active'
 ORDER BY sim DESC LIMIT 3;
```

### Supersession preserves history

A correction NEVER overwrites or deletes. The new row points back via `supersedes`; the old row
is marked `superseded` with `superseded_by` set. Normal retrieval (filtering `active`) shows only
current truth; the version chain stays walkable:

```sql
WITH RECURSIVE history AS (
  SELECT * FROM memories WHERE id = $1
  UNION ALL
  SELECT m.* FROM memories m JOIN history h ON m.id = h.supersedes
)
SELECT * FROM history ORDER BY version;
```

**News/event verification** fits here: store first report as `confidence='low'`; when a source
verifies/corrects, insert the verified version superseding it at `confidence='high'`. Keep the
original linked — "reported then revised" is itself information.

## 6. The background sweep (the GC itself)

One `time.Ticker` job runs all heavy work off the hot path. Passes, in order:

1. **Dedup pass** — trigram over recently-inserted rows; collapse near-dupes by reinforcing the
   survivor (bump its `access_count` + `last_accessed_at`, optionally raise `importance`), and
   marking the loser `dead` with reason `'duplicate'`. (Optional — add when redundancy is measured.)
2. **Supersession pass** — resolve flagged contradictions (mostly handled at structured-fact
   ingest; this catches raw-text cases).
3. **Archive pass** — archive `project` rows scoring below threshold. Global excluded by `WHERE`.
4. **Purge pass** — destructive; deletes rows `dead` for 30d+. Gated behind `GC_PURGE_ENABLED`.

Every transition writes to `memory_audit`. Example archive pass:

```sql
WITH moved AS (
  UPDATE memories
     SET status = 'archived', archived_at = now()
   WHERE scope = 'project' AND status = 'active' AND NOT pinned
     AND importance * exp(-0.05 * extract(epoch FROM now()-last_accessed_at)/86400
                          / (1 + access_count)) < 0.1
  RETURNING id
)
INSERT INTO memory_audit(memory_id, old_status, new_status, reason)
SELECT id, 'active', 'archived', 'decay' FROM moved;
```

**Why this satisfies "least delay, least pressure":** ingest is a plain INSERT (no comparison on
write); reads are a ranked SELECT plus a tiny reinforcement UPDATE; all trigram/decay work is
confined to this scheduled job. A transient duplicate may briefly exist until the sweep collapses
it — harmless for a memory store.

## 7. Observability

- `memory_audit` answers "why did this row change?" for any row.
- A dashboard query of counts grouped by `(status, scope)` over time shows GC behavior at a glance
  and is how you tune `lambda` / thresholds.

```sql
SELECT scope, status, count(*) FROM memories GROUP BY scope, status ORDER BY scope, status;
```

## 8. Non-negotiables

- **Scope gate** on every decay/archive query (`WHERE scope = 'project'`).
- **Soft-delete** — GC never `DELETE`s except the gated purge pass on long-`dead` rows.
- **No external calls** in the comparison path. Structured lookup + trigram + "keep both" only.

## 9. Out of scope (do NOT build unless asked)

- Embeddings / `pgvector` semantic dedup — new dependency, opaque, risky rollback. Not needed for
  a BM25 system unless *semantic paraphrase* redundancy is a measured problem.
- LLM summarization/consolidation — lossy, hard to trace. Last resort only.
- Separate physical tables / partitioning — a scale optimization; the `scope` column is what you'd
  partition on later, so nothing is wasted by starting single-table.
