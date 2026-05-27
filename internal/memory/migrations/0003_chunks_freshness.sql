-- Phase 2: support freshness filters cheaply.
-- All freshness columns (published_at, valid_from, valid_to, superseded_by)
-- already exist from 0001_chunks_core.sql; this migration only adds the
-- supporting indexes.
CREATE INDEX IF NOT EXISTS chunks_published_at
    ON chunks (published_at);

CREATE INDEX IF NOT EXISTS chunks_live
    ON chunks (id) WHERE superseded_by IS NULL;
