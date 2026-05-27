-- ParadeDB pg_search BM25 (Tantivy-backed) on chunks(content).
-- pg_search requires a key_field; we use the primary key.
-- IF NOT EXISTS keeps re-runs idempotent (matches Phase 0 migration style).
CREATE EXTENSION IF NOT EXISTS pg_search;

CREATE INDEX IF NOT EXISTS chunks_content_bm25
    ON chunks
    USING bm25 (id, content)
    WITH (key_field='id');
