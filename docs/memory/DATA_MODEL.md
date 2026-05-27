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
- `0004_topic_digests` — optional, Phase 3.
