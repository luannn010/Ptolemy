-- Phase 5: correction/redundancy layer.
-- pg_trgm was deferred from Phase 4 (NOT enabled there); enable it now.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Trigram GIN index for dedupRecent()'s similarity()/% candidate prefilter.
CREATE INDEX IF NOT EXISTS chunks_content_trgm
    ON chunks USING gin (content gin_trgm_ops);

-- Structured-fact ladder lookup (SPEC §5 step 1: same subject+predicate).
CREATE INDEX IF NOT EXISTS chunks_fact
    ON chunks (fact_subject, fact_predicate)
    WHERE fact_subject IS NOT NULL AND fact_predicate IS NOT NULL;

-- Unification backfill: bring legacy Phase-2-superseded rows (superseded_by set,
-- status still 'active') onto the unified status model so retrieval can rely on
-- status='active' alone. Idempotent; within the existing chunks_status_chk CHECK.
UPDATE chunks SET status = 'superseded'
 WHERE superseded_by IS NOT NULL AND status = 'active';
