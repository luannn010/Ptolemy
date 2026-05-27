CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS chunks (
    id            TEXT PRIMARY KEY,
    content       TEXT NOT NULL,
    embedding     VECTOR(__EMBEDDING_DIM__),
    metadata      JSONB NOT NULL DEFAULT '{}',
    source        TEXT,
    tenant_id     TEXT,
    published_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_from    TIMESTAMPTZ,
    valid_to      TIMESTAMPTZ,
    superseded_by TEXT REFERENCES chunks(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS chunks_embedding_hnsw
    ON chunks USING hnsw (embedding vector_cosine_ops);

CREATE TABLE IF NOT EXISTS memory_schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
