-- Phase 6a: conversational-memory scoping columns on chunks.
-- subject_id = user id (global across sessions); session_id + project_id tag the
-- capture context; perspective types the entry. Global KB rows leave all NULL.
ALTER TABLE chunks
  ADD COLUMN IF NOT EXISTS subject_id  TEXT,
  ADD COLUMN IF NOT EXISTS session_id  TEXT,
  ADD COLUMN IF NOT EXISTS project_id  TEXT,
  ADD COLUMN IF NOT EXISTS perspective TEXT;

DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_perspective_chk
    CHECK (perspective IS NULL OR perspective IN ('factual','relational'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ISOLATION INVARIANT: a project row MUST be owned by a subject. Without this a
-- capture bug inserting subject_id=NULL produces a project row that matches the
-- `subject_id IS NULL` arm of recall and leaks to EVERY user.
DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_project_owned_chk
    CHECK (scope = 'global' OR subject_id IS NOT NULL);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Recall filter index (project rows only; global KB unaffected).
CREATE INDEX IF NOT EXISTS chunks_subject ON chunks (subject_id, project_id)
  WHERE subject_id IS NOT NULL;
