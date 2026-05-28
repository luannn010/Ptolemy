# Data Model (PostgreSQL)

One store holds the text, the vectors, and both search indexes. No separate search
service. Columns are introduced by phase so the schema grows with the build.

> Replace `EMBEDDING_DIM` with your embedding model's actual dimension (record it during
> the prerequisite check). Replace index/opclass names if your BM25 backend differs
> (see `RETRIEVAL.md`).

---

## Extensions

```sql
CREATE EXTENSION IF NOT EXISTS vector;        -- pgvector (Phase 0)
CREATE EXTENSION IF NOT EXISTS pg_textsearch; -- BM25     (Phase 1; host-dependent)
```

---

## Table: `chunks`

```sql
CREATE TABLE chunks (
    -- Phase 0 (core)
    id            TEXT PRIMARY KEY,
    content       TEXT NOT NULL,
    embedding     VECTOR(EMBEDDING_DIM),         -- set by Embedder
    metadata      JSONB NOT NULL DEFAULT '{}',
    source        TEXT,
    tenant_id     TEXT,                           -- multi-tenant isolation (nullable if single-tenant)

    -- Phase 2 (freshness)
    published_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_from    TIMESTAMPTZ,
    valid_to      TIMESTAMPTZ,
    superseded_by TEXT REFERENCES chunks(id),     -- NULL = current/live

    -- Phase 3 (optional enrichment)
    perspective   TEXT,                           -- e.g. 'factual' | 'relational'

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Indexes

```sql
-- Phase 0: dense vector index (pgvector / HNSW). Use the opclass matching your distance.
CREATE INDEX chunks_embedding_hnsw
    ON chunks USING hnsw (embedding vector_cosine_ops);

-- Phase 1: BM25 keyword index (pg_textsearch).
CREATE INDEX chunks_content_bm25
    ON chunks USING bm25 (content) WITH (text_config = 'english');

-- Phase 2: support freshness filters cheaply.
CREATE INDEX chunks_published_at  ON chunks (published_at);
CREATE INDEX chunks_live          ON chunks (id) WHERE superseded_by IS NULL;

-- If multi-tenant, index tenant_id (and consider it as the leading column above).
CREATE INDEX chunks_tenant        ON chunks (tenant_id);
```

### Column notes

- `embedding` — dimension MUST match the embedding model. Re-embedding the whole table is
  required if you ever change models; keep that in mind for `RecencyTtlJob`.
- `published_at` — the freshness clock. For ingested documents, set it from the source's
  real publish date when known, not ingestion time, or recency ranking will be wrong.
- `valid_from` / `valid_to` — optional. Use when a fact is only true for a window.
- `superseded_by` — the supersession pointer. A row with a non-NULL value is stale and is
  filtered out of retrieval by default (see `RETRIEVAL.md`). Never hard-delete; retiring
  is reversible and auditable.
- `tenant_id` — if Ptolemy is multi-tenant, every retrieval query MUST filter on it.

### Phase 4 (GC lifecycle)

```sql
-- Migration 0004_chunks_gc_lifecycle adds the following columns to chunks:
ALTER TABLE chunks ADD COLUMN scope TEXT DEFAULT 'global';                  -- 'project' decays; 'global' is immune
ALTER TABLE chunks ADD COLUMN status TEXT DEFAULT 'active';                 -- active | archived | superseded | dead
ALTER TABLE chunks ADD COLUMN importance REAL DEFAULT 0.5;                  -- [0,1], influence on decay
ALTER TABLE chunks ADD COLUMN pinned BOOLEAN DEFAULT false;                 -- pinned rows are decay-immune
ALTER TABLE chunks ADD COLUMN access_count INTEGER DEFAULT 0;               -- reads reinforce; bumped on every Answer
ALTER TABLE chunks ADD COLUMN last_accessed_at TIMESTAMPTZ;                 -- reinforcement signal
ALTER TABLE chunks ADD COLUMN confidence TEXT DEFAULT 'normal';             -- low | normal | high (Phase 5 news)
ALTER TABLE chunks ADD COLUMN version INTEGER NOT NULL DEFAULT 1;           -- Phase 5 supersession chain (starts at 1, incremented by Supersede)
ALTER TABLE chunks ADD COLUMN supersedes TEXT REFERENCES chunks(id);        -- forward pointer for dedup
ALTER TABLE chunks ADD COLUMN archived_at TIMESTAMPTZ;                      -- transition timestamp
ALTER TABLE chunks ADD COLUMN dead_at TIMESTAMPTZ;                          -- anchors purge grace (30d from dead_at)
ALTER TABLE chunks ADD COLUMN fact_subject TEXT;                            -- Phase 5 structured-fact dedup
ALTER TABLE chunks ADD COLUMN fact_predicate TEXT;                          -- Phase 5 structured-fact dedup

-- Indexes for GC:
CREATE INDEX chunks_status_active ON chunks (id)        WHERE status = 'active';  -- retrieval filtering
CREATE INDEX chunks_scope_status  ON chunks (scope, status);                 -- decay query filtering
CREATE INDEX chunk_audit_chunk_id ON chunk_audit (chunk_id);                 -- audit trail lookups

-- Phase 5 adds: pg_trgm extension + chunks_content_trgm GIN index + chunks_fact partial index
-- (see migration 0005). A partial index on dead_at WHERE status='dead' is deferred to Phase 6
-- when dedup starts producing many dead rows in need of efficient purge sweeps.
```

**Column notes**

- `scope` — 'project' rows decay over time; 'global' rows are immune. The archive query's `scope='project'`
  clause enforces this boundary (schema-level firewall). A row's scope determines whether its decay score
  is calculated at all.
- `status` — lifecycle state: 'active' (returned by retrieval), 'archived' (off-path until unarchived),
  'superseded' (replaced by a newer version), 'dead' (in purge grace period). Retrieval filters `WHERE
  status='active'`. Transition audited in `chunk_audit`.
- `importance` / `pinned` — `pinned=true` exempts the row from decay entirely; `importance` scales the
  decay rate for non-pinned rows (Phase 5 tuning). Pinned + global rows are decay-immune.
- `access_count` / `last_accessed_at` — reinforcement signals bumped on every read (via `Answer`). Hot
  path does not audit; only status transitions are audited to `chunk_audit`.
- `confidence` — 'low' | 'normal' | 'high'. Wired at ingest (Phase 5): set via `RawDocument.Metadata["confidence"]`; flows through the structured-fact ladder so a high-confidence correction supersedes a low-confidence first report.
- `version` / `supersedes` — Phase 5 supersession chain. `version` is an INTEGER (starts 1, incremented by `Supersede`). `supersedes` is the backward pointer to the retired chunk; `superseded_by` on the old row is the forward pointer. Both are LIVE — used by `Supersede()`, `History()`, and the structured-fact ingest ladder.
- `archived_at` / `dead_at` — transition timestamps. `dead_at` is the anchor for the purge grace period
  (e.g., 30 days: any row with `status='dead' AND dead_at <= now() - INTERVAL '30 days'` is eligible
  for purge).
- `fact_subject` / `fact_predicate` — Phase 5 structured-fact dedup. LIVE — used by the ingest ladder: when both are set, `Orchestrator.Ingest` calls `LookupFact` to detect duplicates (→ `Reinforce`) or contradictions (→ `Supersede`). Requires `chunks_fact` partial index (migration 0005).

### Table: `chunk_audit`

```sql
CREATE TABLE chunk_audit (
    id          TEXT PRIMARY KEY,              -- uuid
    chunk_id    TEXT NOT NULL REFERENCES chunks(id),
    old_status  TEXT,                          -- status before the transition
    new_status  TEXT NOT NULL,                 -- status after the transition
    reason      TEXT,                          -- decay, manual, supersession, etc.
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX chunk_audit_chunk_id ON chunk_audit (chunk_id);
CREATE INDEX chunk_audit_created_at ON chunk_audit (created_at);
```

Append-only trail of every status transition (e.g., active → archived by decay, or archived → dead by aging).
Used for observability and reversibility. Reinforcement (access_count bumps) is **not** audited (hot path).

---

## Table: `topic_digests` (Phase 3, optional)

Holds the "evolving topic summary" produced by `DigestSynthesisJob`. Skip entirely unless
Phase 3 synthesis is adopted.

```sql
CREATE TABLE topic_digests (
    id          TEXT PRIMARY KEY,
    topic       TEXT NOT NULL,
    summary     TEXT NOT NULL,
    embedding   VECTOR(EMBEDDING_DIM),
    source_ids  TEXT[] NOT NULL DEFAULT '{}',     -- chunks that fed this digest
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX topic_digests_embedding_hnsw
    ON topic_digests USING hnsw (embedding vector_cosine_ops);
```

> Important: the StructMem paper this idea comes from has **no conflict-resolution
> mechanism** — it accumulates summaries without retiring stale ones. The
> `DigestSynthesisJob` MUST add a revision step that reconciles the new summary against
> the previous one (and against `superseded_by` on the source chunks), or digests will
> drift out of date. Do not ship synthesis without this.

---

## Migrations

Create one migration per phase so the schema's growth mirrors the build:

- `0001_chunks_core` — table + `embedding` + HNSW index.
- `0002_chunks_bm25` — BM25 index (+ extension).
- `0003_chunks_freshness` — `published_at`, `valid_*`, `superseded_by`, freshness indexes.
- `0004_chunks_gc_lifecycle` — GC lifecycle columns (`scope`, `status`, `importance`, `pinned`,
  `access_count`, `last_accessed_at`, `confidence`, `version`, `supersedes`, `archived_at`,
  `dead_at`, `fact_subject`, `fact_predicate`), `chunk_audit` table, GC indexes.
- `0005_chunks_dedup_supersession` — Phase 5: `CREATE EXTENSION pg_trgm`; `chunks_content_trgm`
  GIN trigram index (dedup candidate prefilter); `chunks_fact` partial composite index on
  `(fact_subject, fact_predicate) WHERE fact_subject IS NOT NULL AND fact_predicate IS NOT NULL`
  (structured-fact ladder lookup); unification backfill
  (`UPDATE ... SET status='superseded' WHERE superseded_by IS NOT NULL AND status='active'`)
  to align legacy Phase-2-superseded rows with the unified `status='active'` retrieval filter.
