-- Phase 4: Memory GC lifecycle columns on chunks + chunk_audit trail.
-- Existing rows backfill via column DEFAULTs: scope='global', status='active',
-- importance=1.0, access_count=0, last_accessed_at=now(), confidence='normal',
-- version=1.
ALTER TABLE chunks
  ADD COLUMN IF NOT EXISTS scope            TEXT NOT NULL DEFAULT 'global',
  ADD COLUMN IF NOT EXISTS status           TEXT NOT NULL DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS importance       REAL NOT NULL DEFAULT 1.0,
  ADD COLUMN IF NOT EXISTS pinned           BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS access_count     INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS confidence       TEXT NOT NULL DEFAULT 'normal',
  ADD COLUMN IF NOT EXISTS version          INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS supersedes       TEXT,
  ADD COLUMN IF NOT EXISTS archived_at      TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS dead_at          TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS fact_subject     TEXT,
  ADD COLUMN IF NOT EXISTS fact_predicate   TEXT;

DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_scope_chk CHECK (scope IN ('project','global'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_status_chk CHECK (status IN ('active','archived','superseded','dead'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_confidence_chk CHECK (confidence IN ('low','normal','high'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS chunks_status_active ON chunks (id)        WHERE status = 'active';
CREATE INDEX IF NOT EXISTS chunks_scope_status  ON chunks (scope, status);

CREATE TABLE IF NOT EXISTS chunk_audit (
    id         BIGSERIAL PRIMARY KEY,
    chunk_id   TEXT NOT NULL,
    old_status TEXT,
    new_status TEXT NOT NULL,
    reason     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chunk_audit_chunk_id ON chunk_audit (chunk_id);
