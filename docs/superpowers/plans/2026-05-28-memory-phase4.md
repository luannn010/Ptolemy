# Memory Module — Phase 4 (Memory GC Core) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a status-lifecycle GC layer to the memory store — scope-tagged schema, reinforcement-on-read, observability, and a dormant background sweep — built and tested now, fully exercised once Phase 6 produces `project` rows.

**Architecture:** Extend `chunks` (no parallel table) via migration `0004` with the full Phase 4–6 column set + a `chunk_audit` trail. Add `Store.Reinforce`/`Stats`, a pure `decayScore`, a `status='active'` retrieval filter, and a `Sweeper` (archiveDecayed + gated purgeDead) launched as an opt-in `workerd` goroutine gated by `DATABASE_URL` + `GC_SWEEP_ENABLED`.

**Tech Stack:** Go 1.25, `pgx/v5`, `pgvector-go`, ParadeDB `pg_search`, zerolog. Live PG at `192.168.0.164:1091`, embed `:1089`, brain `:1090`.

**Plan refinements vs. spec (semantics unchanged):**
- `PgStore.Upsert` grows to write `scope` (Go-side default `'global'` when `Chunk.Scope == ""`), so the `Metadata["scope"]` path works. All other GC columns ride their column DEFAULTs on insert.
- `PgStore.Get` and the retriever `Scan` are NOT extended with GC columns — nothing in Phase 4 reads them through those paths (sweep queries chunks directly; the status filter is a WHERE clause). Avoids scope creep.
- `MaybeStartSweep(ctx) (cleanup func(), enabled bool, err error)` — the goroutine stops when `ctx` is cancelled; `cleanup` closes the pgx conn. workerd cancels the ctx then calls cleanup on shutdown.

---

## File map

**Created:**
- `internal/memory/migrations/0004_chunks_gc_lifecycle.sql` — columns + constraints + 2 indexes + `chunk_audit`.
- `internal/memory/score.go` — `decayScore()` pure function.
- `internal/memory/score_test.go` — decay unit tests + Go-vs-SQL agreement test.
- `internal/memory/sweep.go` — `Sweeper`, `GCConfig`, `Run`, `sweepOnce`, `archiveDecayed`, `purgeDead`, `runLoop`.
- `internal/memory/sweep_test.go` — firewall acceptance, reversibility, purge gating, runLoop tolerance.

**Modified:**
- `internal/memory/types.go` — lifecycle fields on `Chunk`.
- `internal/memory/migrations_test.go` — FS-contains + columns/audit/indexes + exact-backfill-defaults.
- `internal/memory/config.go` — `GCConfig` + env parsing (`boolEnv`, `durationEnv`) + validation.
- `internal/memory/config_test.go` — GC defaults + validation tests.
- `internal/memory/store.go` — `Reinforce`, `Stats`, `ScopeStatusCount`; `Upsert` writes `scope`.
- `internal/memory/store_test.go` — `Reinforce`/`Stats` integration.
- `internal/memory/store_unit_test.go` — fake store records `Reinforce`/`Stats`.
- `internal/memory/hybrid_retriever.go` — `AND status='active'` in both CTEs.
- `internal/memory/hybrid_retriever_test.go` — `TestHybridRetriever_ExcludesNonActive`.
- `internal/memory/bm25_retriever.go` — `AND status='active'`.
- `internal/memory/orchestrator.go` — `Answer` calls `Reinforce`; `Ingest` reads `Metadata["scope"]`.
- `internal/memory/orchestrator_test.go` — reinforce + scope tests.
- `internal/memory/module.go` — `MaybeStartSweep`.
- `internal/memory/module_test.go` — `MaybeStartSweep` disabled-path test.
- `cmd/workerd/main.go` — call `MaybeStartSweep`; cancel + cleanup on shutdown.
- `.env.example` — 6 GC env vars.
- `docs/memory/DATA_MODEL.md` — lifecycle columns + `chunk_audit`.
- `docs/memory/IMPLEMENTATION_PLAN.md` — Phase 4 section.

**Committed as reference (untracked → tracked):**
- `docs/memory/{PHASE_4_MEMORY_GC_CORE,PHASE_5_MEMORY_GC_SUPERSESSION_DEDUP,PHASE_6_CONVERSATIONAL_MEMORY,README-GC,SPEC-GC,TASKS-GC}.md`

---

## Task 1: Migration 0004 (schema + audit)

**Files:**
- Create: `internal/memory/migrations/0004_chunks_gc_lifecycle.sql`
- Test: `internal/memory/migrations_test.go`

- [ ] **Step 1.1: Write the migration SQL**

Create `internal/memory/migrations/0004_chunks_gc_lifecycle.sql`. CHECK constraints are wrapped in `DO` blocks so a partial-failure re-run (the runner records success only on full completion) doesn't error on "constraint already exists":

```sql
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
```

- [ ] **Step 1.2: Write the failing FS test**

Append to `internal/memory/migrations_test.go`:

```go
func TestMigrationsFS_Contains0004(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0004_chunks_gc_lifecycle.sql")
	if err != nil {
		t.Fatalf("0004 migration missing from embed FS: %v", err)
	}
	if !strings.Contains(string(data), "chunk_audit") {
		t.Fatalf("0004 should create chunk_audit")
	}
}
```

(`strings` is already imported in this test file; if not, add it.)

- [ ] **Step 1.3: Run the FS test to verify it fails**

Run: `go test ./internal/memory/ -run TestMigrationsFS_Contains0004 -v`
Expected: FAIL — `0004 migration missing from embed FS` (file not created yet, or embed not refreshed). After creating the file in 1.1 it should PASS; if it already passes, good.

- [ ] **Step 1.4: Write the integration test for columns + backfill defaults**

Append to `internal/memory/migrations_test.go`:

```go
func TestApplyMigrations_GCLifecycleColumns(t *testing.T) {
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())

	// Fresh schema.
	_, _ = conn.Exec(context.Background(),
		`DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE`)
	if err := ApplyMigrations(context.Background(), conn, 4); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	// chunk_audit exists.
	var auditExists bool
	if err := conn.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='chunk_audit')`,
	).Scan(&auditExists); err != nil {
		t.Fatal(err)
	}
	if !auditExists {
		t.Fatal("chunk_audit table not created")
	}

	// Both indexes exist.
	for _, idx := range []string{"chunks_status_active", "chunks_scope_status"} {
		var exists bool
		if err := conn.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname=$1)`, idx,
		).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("index %s not created", idx)
		}
	}

	// Backfill defaults: insert a row writing ONLY pre-GC columns; the GC
	// columns must take their DEFAULTs. This is exactly what existing rows get.
	_, err = conn.Exec(context.Background(), `
		INSERT INTO chunks (id, content, embedding, metadata, published_at)
		VALUES ('bf1', 'x', NULL, '{}', now())
	`)
	if err != nil {
		t.Fatalf("insert pre-GC row: %v", err)
	}
	var (
		scope, status, confidence string
		importance                float64
		pinned                    bool
		accessCount, version      int
		lastAccessed              time.Time
	)
	if err := conn.QueryRow(context.Background(), `
		SELECT scope, status, importance, pinned, access_count, last_accessed_at,
		       confidence, version
		FROM chunks WHERE id='bf1'
	`).Scan(&scope, &status, &importance, &pinned, &accessCount, &lastAccessed,
		&confidence, &version); err != nil {
		t.Fatal(err)
	}
	if scope != "global" || status != "active" || importance != 1.0 || pinned ||
		accessCount != 0 || confidence != "normal" || version != 1 {
		t.Fatalf("backfill defaults wrong: scope=%q status=%q imp=%v pinned=%v ac=%d conf=%q ver=%d",
			scope, status, importance, pinned, accessCount, confidence, version)
	}
	if time.Since(lastAccessed) > 5*time.Second {
		t.Fatalf("last_accessed_at should default to ~now(), got %v ago", time.Since(lastAccessed))
	}
}
```

Add imports `context`, `time`, and `github.com/jackc/pgx/v5` to the test file if not present (check the existing import block).

- [ ] **Step 1.5: Run the integration test against the live DB**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run TestApplyMigrations_GCLifecycleColumns -v
```
Expected: PASS. (Without `DATABASE_URL`, `requirePG` skips it cleanly.)

- [ ] **Step 1.6: Commit**

```bash
git add internal/memory/migrations/0004_chunks_gc_lifecycle.sql internal/memory/migrations_test.go
git commit -m "$(cat <<'EOF'
feat(memory): migration 0004 adds GC lifecycle columns + chunk_audit

Extends chunks with the full Phase 4-6 column set (scope, status,
importance, pinned, access_count, last_accessed_at, confidence, version,
supersedes, archived_at, dead_at, fact_subject, fact_predicate), three
CHECK constraints (DO-block idempotent), two indexes (partial status=active
+ scope,status), and the chunk_audit trail. Existing rows backfill via
column DEFAULTs to global/active/1.0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Chunk struct lifecycle fields

**Files:**
- Modify: `internal/memory/types.go`

- [ ] **Step 2.1: Add the fields**

In `internal/memory/types.go`, extend the `Chunk` struct (after the existing `CreatedAt` field, keeping all existing fields):

```go
	// Phase 4 (GC lifecycle)
	Scope          string     // 'project' | 'global'
	Status         string     // 'active' | 'archived' | 'superseded' | 'dead'
	Importance     float64
	Pinned         bool
	AccessCount    int
	LastAccessedAt time.Time
	Confidence     string     // 'low' | 'normal' | 'high'
	Version        int
	Supersedes     *string
	ArchivedAt     *time.Time
	DeadAt         *time.Time
	FactSubject    *string
	FactPredicate  *string
```

- [ ] **Step 2.2: Verify it compiles**

Run: `go build ./internal/memory/`
Expected: success (no behavior change; fields are additive).

- [ ] **Step 2.3: Commit**

```bash
git add internal/memory/types.go
git commit -m "$(cat <<'EOF'
feat(memory): Chunk gains GC lifecycle fields

Mirrors the 0004 columns. Additive; existing Phase 0-3 fields untouched.
Populated by ingest (Scope) and the sweep; most are dormant until Phase 5/6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: decayScore (pure function)

**Files:**
- Create: `internal/memory/score.go`
- Create: `internal/memory/score_test.go`

- [ ] **Step 3.1: Write the failing unit tests**

Create `internal/memory/score_test.go`:

```go
package memory

import (
	"math"
	"testing"
)

func TestDecayScore_GlobalAndPinnedAreImmune(t *testing.T) {
	// global scope → 1.0 regardless of age/access.
	if got := decayScore(0.5, "global", false, 0, 365, 0.05); got != 1.0 {
		t.Fatalf("global should be 1.0, got %v", got)
	}
	// pinned project → 1.0 regardless of age.
	if got := decayScore(0.5, "project", true, 0, 365, 0.05); got != 1.0 {
		t.Fatalf("pinned should be 1.0, got %v", got)
	}
}

func TestDecayScore_DecaysWithAge(t *testing.T) {
	young := decayScore(1.0, "project", false, 0, 1, 0.05)
	old := decayScore(1.0, "project", false, 0, 100, 0.05)
	if !(old < young) {
		t.Fatalf("older project row must score lower: young=%v old=%v", young, old)
	}
	if young > 1.0 || old < 0 {
		t.Fatalf("score out of [0,1] range: young=%v old=%v", young, old)
	}
}

func TestDecayScore_ReinforcementFlattens(t *testing.T) {
	// Same age, more accesses → higher score (curve flattens).
	cold := decayScore(1.0, "project", false, 0, 30, 0.05)
	hot := decayScore(1.0, "project", false, 50, 30, 0.05)
	if !(hot > cold) {
		t.Fatalf("reinforcement should raise score: cold=%v hot=%v", cold, hot)
	}
}

func TestDecayScore_Formula(t *testing.T) {
	// Spot-check the exact formula for a project row.
	got := decayScore(0.8, "project", false, 3, 10, 0.05)
	want := 0.8 * math.Exp(-0.05*10/(1+3))
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("formula mismatch: got %v want %v", got, want)
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

Run: `go test ./internal/memory/ -run TestDecayScore -v`
Expected: FAIL — `undefined: decayScore` (compile error).

- [ ] **Step 3.3: Implement decayScore**

Create `internal/memory/score.go`:

```go
package memory

import "math"

// decayScore computes a memory's retention score. Global-scope or pinned rows
// are decay-immune (always 1.0) — enforced here AND by the WHERE clause in the
// archive sweep, so immunity does not depend on a tunable.
//
// For decaying ('project', not pinned) rows:
//
//	score = importance * exp( -lambda * daysSinceAccess / (1 + accessCount) )
//
// Reinforcement (high accessCount) flattens the curve so frequently-used
// memories persist. The archive sweep inlines this same formula in SQL;
// TestDecayScore_GoMatchesSQL guards the two against drift.
func decayScore(importance float64, scope string, pinned bool, accessCount int, daysSinceAccess, lambda float64) float64 {
	if scope == "global" || pinned {
		return 1.0
	}
	return importance * math.Exp(-lambda*daysSinceAccess/(1+float64(accessCount)))
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run TestDecayScore -v`
Expected: PASS (4 tests; the Go-vs-SQL agreement test is added in Task 6).

- [ ] **Step 3.5: Commit**

```bash
git add internal/memory/score.go internal/memory/score_test.go
git commit -m "$(cat <<'EOF'
feat(memory): decayScore pure function

score = importance * exp(-lambda * days / (1+access_count)); global/pinned
rows are decay-immune (1.0). The archive sweep inlines the same formula in
SQL; a Go-vs-SQL agreement test (Task 6) guards the duplication.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: GCConfig + env parsing + validation

**Files:**
- Modify: `internal/memory/config.go`
- Test: `internal/memory/config_test.go`

- [ ] **Step 4.1: Write the failing tests**

Append to `internal/memory/config_test.go` (the existing `setRequiredEnv` pattern uses `t.Setenv` for the six base vars; replicate it):

```go
func TestLoadConfig_GCDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	for _, k := range []string{"GC_SWEEP_ENABLED", "GC_SWEEP_INTERVAL", "GC_DECAY_LAMBDA",
		"GC_ARCHIVE_THRESHOLD", "GC_PURGE_ENABLED", "GC_PURGE_GRACE_DAYS"} {
		t.Setenv(k, "")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GC.SweepEnabled || cfg.GC.PurgeEnabled {
		t.Fatalf("enables should default false: %+v", cfg.GC)
	}
	if cfg.GC.SweepInterval != time.Hour {
		t.Fatalf("interval default 1h, got %v", cfg.GC.SweepInterval)
	}
	if cfg.GC.DecayLambda != 0.05 || cfg.GC.ArchiveThreshold != 0.1 || cfg.GC.PurgeGraceDays != 30 {
		t.Fatalf("GC numeric defaults wrong: %+v", cfg.GC)
	}
}

func TestLoadConfig_GCEnvParsed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("GC_SWEEP_ENABLED", "true")
	t.Setenv("GC_SWEEP_INTERVAL", "5s")
	t.Setenv("GC_DECAY_LAMBDA", "0.1")
	t.Setenv("GC_ARCHIVE_THRESHOLD", "0.2")
	t.Setenv("GC_PURGE_ENABLED", "true")
	t.Setenv("GC_PURGE_GRACE_DAYS", "7")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GC.SweepEnabled || !cfg.GC.PurgeEnabled {
		t.Fatalf("enables should be true: %+v", cfg.GC)
	}
	if cfg.GC.SweepInterval != 5*time.Second {
		t.Fatalf("interval: got %v", cfg.GC.SweepInterval)
	}
	if cfg.GC.DecayLambda != 0.1 || cfg.GC.ArchiveThreshold != 0.2 || cfg.GC.PurgeGraceDays != 7 {
		t.Fatalf("GC numerics: %+v", cfg.GC)
	}
}

func TestLoadConfig_GCRejectsBadValues(t *testing.T) {
	base := func() {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("EMBEDDING_BASE_URL", "http://e")
		t.Setenv("EMBEDDING_MODEL", "m")
		t.Setenv("EMBEDDING_DIM", "1024")
		t.Setenv("BRAIN_BASE_URL", "http://l")
		t.Setenv("BRAIN_MODEL", "lm")
	}
	cases := map[string]map[string]string{
		"negative lambda":   {"GC_DECAY_LAMBDA": "-0.1"},
		"threshold over 1":  {"GC_ARCHIVE_THRESHOLD": "1.5"},
		"threshold zero":    {"GC_ARCHIVE_THRESHOLD": "0"},
		"interval too small": {"GC_SWEEP_INTERVAL": "10s"}, // < 1m floor? see note
		"grace under 1 day": {"GC_PURGE_GRACE_DAYS": "0"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			base()
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := LoadConfig(); err == nil {
				t.Fatalf("%s: expected error", name)
			}
		})
	}
}
```

NOTE on the interval floor: the spec says interval ≥ 1m. But `TestLoadConfig_GCEnvParsed` sets `5s` (so a fast manual workerd test works). **Resolve the contradiction by making the validation floor 1s, not 1m** — a 5s interval is legitimate for testing/dev, and there's no correctness hazard in a short interval (the sweep is idempotent). Update the "interval too small" case to use `"500ms"` (below the 1s floor) instead of `"10s"`. Use this corrected case:

```go
		"interval too small": {"GC_SWEEP_INTERVAL": "500ms"}, // < 1s floor
```

- [ ] **Step 4.2: Run tests to verify they fail**

Run: `go test ./internal/memory/ -run TestLoadConfig_GC -v`
Expected: FAIL — `cfg.GC undefined` (compile error).

- [ ] **Step 4.3: Implement GCConfig + helpers + validation**

In `internal/memory/config.go`:

(a) Add `"time"` to imports if not present (Phase 3 added it; verify).

(b) Add the `GCConfig` type and a `GC` field on `MemoryConfig` (after the recency fields):

```go
type GCConfig struct {
	SweepEnabled     bool          // GC_SWEEP_ENABLED (default false)
	SweepInterval    time.Duration // GC_SWEEP_INTERVAL (default 1h)
	DecayLambda      float64       // GC_DECAY_LAMBDA (default 0.05)
	ArchiveThreshold float64       // GC_ARCHIVE_THRESHOLD (default 0.1)
	PurgeEnabled     bool          // GC_PURGE_ENABLED (default false)
	PurgeGraceDays   float64       // GC_PURGE_GRACE_DAYS (default 30)
}
```

Add `GC GCConfig` to the `MemoryConfig` struct.

(c) In `LoadConfig`, after the recency block, populate + validate GC:

```go
	cfg.GC = GCConfig{
		SweepEnabled:     boolEnv("GC_SWEEP_ENABLED", false),
		SweepInterval:    durationEnv("GC_SWEEP_INTERVAL", time.Hour),
		DecayLambda:      floatEnv("GC_DECAY_LAMBDA", 0.05),
		ArchiveThreshold: floatEnv("GC_ARCHIVE_THRESHOLD", 0.1),
		PurgeEnabled:     boolEnv("GC_PURGE_ENABLED", false),
		PurgeGraceDays:   floatEnv("GC_PURGE_GRACE_DAYS", 30),
	}
	if cfg.GC.DecayLambda < 0 {
		return MemoryConfig{}, fmt.Errorf("GC_DECAY_LAMBDA must be >= 0, got %v", cfg.GC.DecayLambda)
	}
	if cfg.GC.ArchiveThreshold <= 0 || cfg.GC.ArchiveThreshold > 1 {
		return MemoryConfig{}, fmt.Errorf("GC_ARCHIVE_THRESHOLD must be in (0,1], got %v", cfg.GC.ArchiveThreshold)
	}
	if cfg.GC.SweepInterval < time.Second {
		return MemoryConfig{}, fmt.Errorf("GC_SWEEP_INTERVAL must be >= 1s, got %v", cfg.GC.SweepInterval)
	}
	if cfg.GC.PurgeGraceDays < 1 {
		return MemoryConfig{}, fmt.Errorf("GC_PURGE_GRACE_DAYS must be >= 1, got %v", cfg.GC.PurgeGraceDays)
	}
```

(d) Add the two env helpers next to `intEnv`/`floatEnv` at the bottom of the file:

```go
func boolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run 'TestLoadConfig' -v`
Expected: PASS (existing config tests + the 3 new GC tests; the bad-values subtests all error as expected).

- [ ] **Step 4.5: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go
git commit -m "$(cat <<'EOF'
feat(memory): GCConfig knobs + boolEnv/durationEnv helpers

GC_SWEEP_ENABLED (false), GC_SWEEP_INTERVAL (1h), GC_DECAY_LAMBDA (0.05),
GC_ARCHIVE_THRESHOLD (0.1), GC_PURGE_ENABLED (false), GC_PURGE_GRACE_DAYS (30).
Validation: lambda>=0, threshold in (0,1], interval>=1s, grace>=1. Defaults
are PLACEHOLDERS to be tuned against real chat volume once Phase 6 lands.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Store.Reinforce + Stats

**Files:**
- Modify: `internal/memory/store.go`
- Test: `internal/memory/store_test.go`, `internal/memory/store_unit_test.go`

- [ ] **Step 5.1: Write the failing integration tests**

Append to `internal/memory/store_test.go` (uses the existing `freshDB(t)` helper):

```go
func TestPgStore_Reinforce_BumpsCounters(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "r1", Content: "x", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	_ = conn.QueryRow(context.Background(),
		`SELECT last_accessed_at FROM chunks WHERE id='r1'`).Scan(&before)

	if err := s.Reinforce(context.Background(), []string{"r1"}); err != nil {
		t.Fatalf("Reinforce: %v", err)
	}

	var count int
	var after time.Time
	if err := conn.QueryRow(context.Background(),
		`SELECT access_count, last_accessed_at FROM chunks WHERE id='r1'`).Scan(&count, &after); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("access_count: got %d want 1", count)
	}
	if !after.After(before) {
		t.Fatalf("last_accessed_at not advanced: before=%v after=%v", before, after)
	}
}

func TestPgStore_Reinforce_EmptyIsNoOp(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	if err := s.Reinforce(context.Background(), nil); err != nil {
		t.Fatalf("empty Reinforce should be a no-op, got %v", err)
	}
}

func TestPgStore_Stats_CountsByScopeStatus(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "g1", Content: "a", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
		{ID: "g2", Content: "b", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	// Force a project+archived row directly.
	_, err := conn.Exec(context.Background(),
		`UPDATE chunks SET scope='project', status='archived' WHERE id='g2'`)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	got := map[string]int{}
	for _, sc := range stats {
		got[sc.Scope+"/"+sc.Status] = sc.Count
	}
	if got["global/active"] != 1 || got["project/archived"] != 1 {
		t.Fatalf("stats wrong: %+v", got)
	}
}
```

- [ ] **Step 5.2: Run tests to verify they fail**

Run: `go test ./internal/memory/ -run 'TestPgStore_Reinforce|TestPgStore_Stats' -v`
Expected: FAIL — `s.Reinforce undefined` / `s.Stats undefined`.

- [ ] **Step 5.3: Implement Reinforce, Stats, ScopeStatusCount + interface**

In `internal/memory/store.go`:

(a) Add to the `Store` interface (after `SupersedeOnUpsert`):

```go
	// Reinforce bumps access_count + last_accessed_at for the given ids in one
	// statement. Empty ids is a no-op. Not audited (hot path).
	Reinforce(ctx context.Context, ids []string) error

	// Stats returns row counts grouped by (scope, status) for observability.
	Stats(ctx context.Context) ([]ScopeStatusCount, error)
```

(b) Add the result type near the top (after the interface):

```go
type ScopeStatusCount struct {
	Scope  string
	Status string
	Count  int
}
```

(c) Implement on `PgStore`:

```go
func (s *PgStore) Reinforce(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.conn.Exec(ctx, `
		UPDATE chunks
		SET access_count = access_count + 1, last_accessed_at = now()
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return fmt.Errorf("reinforce: %w", err)
	}
	return nil
}

func (s *PgStore) Stats(ctx context.Context) ([]ScopeStatusCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT scope, status, count(*)
		FROM chunks
		GROUP BY scope, status
		ORDER BY scope, status
	`)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	defer rows.Close()
	var out []ScopeStatusCount
	for rows.Next() {
		var c ScopeStatusCount
		if err := rows.Scan(&c.Scope, &c.Status, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5.4: Update the fake store in store_unit_test.go**

The fake `Store` implementation used by orchestrator tests must satisfy the new interface methods. In `internal/memory/store_unit_test.go`, find the fake store type (it implements `Store`) and add recorder fields + methods. Add fields to the struct:

```go
	reinforced [][]string // records each Reinforce call's ids
	statsCalls int
```

And methods:

```go
func (f *fakeStore) Reinforce(_ context.Context, ids []string) error {
	f.reinforced = append(f.reinforced, ids)
	return nil
}

func (f *fakeStore) Stats(_ context.Context) ([]ScopeStatusCount, error) {
	f.statsCalls++
	return nil, nil
}
```

(Adjust the receiver type name to match the actual fake store in that file — check with `grep -n "func (.*) Upsert" internal/memory/store_unit_test.go`. If the fake lives in `orchestrator_test.go` instead, add the methods there.)

- [ ] **Step 5.5: Run tests to verify they pass**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run 'TestPgStore_Reinforce|TestPgStore_Stats' -v
```
Expected: PASS. Also run `go test ./internal/memory/ -run TestOrchestrator -v` to confirm the fake-store change didn't break compilation.

- [ ] **Step 5.6: Commit**

```bash
git add internal/memory/store.go internal/memory/store_test.go internal/memory/store_unit_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Store.Reinforce + Stats

Reinforce bumps access_count + last_accessed_at in one batched UPDATE
(empty ids = no-op, not audited). Stats returns counts by (scope, status)
for observability/tuning. Fake store records both for orchestrator tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Upsert writes scope + Go-vs-SQL decay agreement test

**Files:**
- Modify: `internal/memory/store.go`
- Test: `internal/memory/score_test.go`

- [ ] **Step 6.1: Write the failing Go-vs-SQL agreement test**

Append to `internal/memory/score_test.go` (this file is `package memory`, so it can use `freshDB`):

```go
import (
	"context"
	"time"
)
// (merge these into the existing import block at the top of score_test.go)

func TestDecayScore_GoMatchesSQL(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	// A project row last accessed 10 days ago, importance 0.8, access_count 3.
	tenDaysAgo := time.Now().UTC().Add(-10 * 24 * time.Hour)
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "d1", Content: "x", Embedding: []float32{1, 0, 0, 0}, PublishedAt: tenDaysAgo},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := conn.Exec(context.Background(), `
		UPDATE chunks SET scope='project', importance=0.8, access_count=3, last_accessed_at=$1
		WHERE id='d1'
	`, tenDaysAgo)
	if err != nil {
		t.Fatal(err)
	}

	const lambda = 0.05
	var sqlScore float64
	if err := conn.QueryRow(context.Background(), `
		SELECT importance * exp(-$1 * extract(epoch FROM now()-last_accessed_at)/86400
		                        / (1 + access_count))
		FROM chunks WHERE id='d1'
	`, lambda).Scan(&sqlScore); err != nil {
		t.Fatal(err)
	}

	// Go side, using the same ~now() reference. The sub-second skew between
	// SQL now() and time.Now() over a 10-day delta is far below 1e-6.
	days := time.Since(tenDaysAgo).Hours() / 24
	goScore := decayScore(0.8, "project", false, 3, days, lambda)

	if math.Abs(goScore-sqlScore) > 1e-6 {
		t.Fatalf("Go/SQL decay mismatch: go=%v sql=%v", goScore, sqlScore)
	}
}
```

- [ ] **Step 6.2: Run it to verify it fails for the right reason**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run TestDecayScore_GoMatchesSQL -v
```
Expected: At this point `Upsert` does not write `scope`, but the test forces `scope='project'` via raw UPDATE, so the test may actually PASS already. That's fine — its purpose is the formula guard, which holds regardless. If it passes, proceed; the Upsert change below is still needed for the ingest path (Task 7).

- [ ] **Step 6.3: Make Upsert write scope (Go-side default 'global')**

In `internal/memory/store.go`, update BOTH the `Upsert` INSERT and the `SupersedeOnUpsert` INSERT to include `scope`. Add a tiny helper and use it. First the helper (near `nullableStr`):

```go
func scopeOrDefault(scope string) string {
	if scope == "" {
		return "global"
	}
	return scope
}
```

Then change the `Upsert` statement's column list + values:

```go
		_, err = s.conn.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at, scope)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				source = EXCLUDED.source,
				tenant_id = EXCLUDED.tenant_id,
				published_at = EXCLUDED.published_at,
				scope = EXCLUDED.scope
		`,
			c.ID, c.Content, pgvector.NewVector(c.Embedding), meta,
			nullableStr(c.Source), nullableStr(c.Tenant), c.PublishedAt, scopeOrDefault(c.Scope),
		)
```

Apply the identical `scope`/`$8`/`EXCLUDED.scope` change to the `SupersedeOnUpsert` INSERT (it has the same statement shape inside the tx).

- [ ] **Step 6.4: Run the agreement test + full store tests**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run 'TestDecayScore|TestPgStore' -v
```
Expected: PASS (agreement test + Reinforce/Stats + existing store tests). The existing `TestPgStore_SupersedeOnUpsert_*` must stay green (scope defaults to 'global', behavior unchanged).

- [ ] **Step 6.5: Commit**

```bash
git add internal/memory/store.go internal/memory/score_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Upsert writes scope (default global) + Go-vs-SQL decay test

Upsert and SupersedeOnUpsert now write the scope column (Go-side default
'global' when Chunk.Scope is empty), enabling the Metadata["scope"] ingest
path. TestDecayScore_GoMatchesSQL asserts the Go decayScore and the inlined
archive-SQL formula agree within 1e-6, guarding the duplication.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Retrieval status='active' filter

**Files:**
- Modify: `internal/memory/hybrid_retriever.go`, `internal/memory/bm25_retriever.go`
- Test: `internal/memory/hybrid_retriever_test.go`

- [ ] **Step 7.1: Write the failing test**

Append to `internal/memory/hybrid_retriever_test.go`:

```go
func TestHybridRetriever_ExcludesNonActive(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()
	chunks := []Chunk{
		{ID: "act", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
		{ID: "arc", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(),
		`UPDATE chunks SET status='archived' WHERE id='arc'`); err != nil {
		t.Fatal(err)
	}
	asOf := now
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, rc := range got {
		if rc.ID == "arc" {
			t.Fatalf("archived chunk must not be retrieved, got %v", idsHybrid(got))
		}
	}
	seenActive := false
	for _, rc := range got {
		if rc.ID == "act" {
			seenActive = true
		}
	}
	if !seenActive {
		t.Fatalf("active chunk should be retrieved, got %v", idsHybrid(got))
	}
}
```

- [ ] **Step 7.2: Run it to verify it fails**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run TestHybridRetriever_ExcludesNonActive -v
```
Expected: FAIL — the archived `arc` row IS retrieved (no status filter yet).

- [ ] **Step 7.3: Add the status filter to both arms**

In `internal/memory/hybrid_retriever.go`, add `AND status = 'active'` to BOTH CTEs (after `superseded_by IS NULL`). The `bm25` CTE:

```sql
    WHERE content @@@ $1
      AND superseded_by IS NULL
      AND status = 'active'
      AND published_at <= $5
```

The `vec` CTE:

```sql
    WHERE superseded_by IS NULL
      AND status = 'active'
      AND published_at <= $5
```

Also add it to the outer `WHERE` (mirrors the existing defensive `superseded_by IS NULL` there):

```sql
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
  AND c.status = 'active'
  AND c.published_at <= $5
```

In `internal/memory/bm25_retriever.go`, add `AND status = 'active'` after `superseded_by IS NULL`:

```sql
		WHERE content @@@ $1
		  AND superseded_by IS NULL
		  AND status = 'active'
		  AND published_at <= $2
```

- [ ] **Step 7.4: Run the test + existing retriever tests**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run 'TestHybridRetriever|TestBm25Retriever' -v
```
Expected: PASS (new test + all existing Phase 1/2/3 retriever tests — existing rows are `active` by default, so nothing else changes).

- [ ] **Step 7.5: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/bm25_retriever.go internal/memory/hybrid_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): retrieval filters status='active' (the firewall's read half)

Both hybrid CTEs + the outer SELECT and the standalone Bm25Retriever gain
AND status='active', so archived/dead/superseded rows vanish from retrieval.
Existing rows are active by default — no behavior change for the live corpus.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Orchestrator — Ingest scope + Answer reinforces

**Files:**
- Modify: `internal/memory/orchestrator.go`
- Test: `internal/memory/orchestrator_test.go`

- [ ] **Step 8.1: Write the failing unit tests**

Append to `internal/memory/orchestrator_test.go`. These use the existing fake store + fake embedder. The scope tests assert the chunk passed to `Upsert` carries the right scope; the reinforce test asserts `Reinforce` is called with the retrieved ids.

```go
func TestOrchestrator_Ingest_ScopeDefaultsGlobal(t *testing.T) {
	fs := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}},
		Store:    fs,
	}
	if err := o.Ingest(context.Background(), RawDocument{ID: "d", Text: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if len(fs.upserted) == 0 || fs.upserted[0].Scope != "global" {
		t.Fatalf("expected scope 'global', got %+v", fs.upserted)
	}
}

func TestOrchestrator_Ingest_ScopeFromMetadata(t *testing.T) {
	fs := &fakeStore{}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}},
		Store:    fs,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d", Text: "alpha", Metadata: map[string]any{"scope": "project"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.upserted) == 0 || fs.upserted[0].Scope != "project" {
		t.Fatalf("expected scope 'project', got %+v", fs.upserted)
	}
}

func TestOrchestrator_Answer_Reinforces(t *testing.T) {
	fs := &fakeStore{}
	o := &Orchestrator{
		Embedder:       fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}},
		Store:          fs,
		Retriever:      &fakeRetriever2{out: []RetrievedChunk{{Chunk: Chunk{ID: "c1#0"}}, {Chunk: Chunk{ID: "c2#0"}}}},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Generator:      fakeGenerator{},
		Depth:          20,
		FinalK:         5,
	}
	if _, err := o.Answer(context.Background(), Query{Text: "alpha", K: 5}); err != nil {
		t.Fatal(err)
	}
	if len(fs.reinforced) != 1 {
		t.Fatalf("expected exactly one Reinforce call, got %d", len(fs.reinforced))
	}
	if len(fs.reinforced[0]) != 2 {
		t.Fatalf("expected 2 ids reinforced, got %v", fs.reinforced[0])
	}
}
```

This needs the fake store to record `upserted []Chunk` (it may already — check; if not, add `upserted` capture to its `Upsert`). It also needs a fake retriever returning fixed chunks and a fake generator. Check `orchestrator_test.go` for existing fakes:
- If a fake retriever returning a fixed list already exists, reuse it (rename `fakeRetriever2` accordingly).
- If `fakeGenerator` doesn't exist, add a minimal one:

```go
type fakeGenerator struct{}
func (fakeGenerator) Generate(_ context.Context, _ Query, _ PromptContext) (Answer, error) {
	return Answer{Text: "ok"}, nil
}
type fakeRetriever2 struct{ out []RetrievedChunk }
func (f *fakeRetriever2) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	return f.out, nil
}
```

And ensure `fakeStore.Upsert` captures chunks:

```go
func (f *fakeStore) Upsert(_ context.Context, chunks []Chunk) error {
	f.upserted = append(f.upserted, chunks...)
	return nil
}
```

(Add the `upserted []Chunk` field to the fake store struct if absent.)

- [ ] **Step 8.2: Run to verify failure**

Run: `go test ./internal/memory/ -run 'TestOrchestrator_Ingest_Scope|TestOrchestrator_Answer_Reinforces' -v`
Expected: FAIL — scope not set (defaults to "" not "global" in the chunk because Chunker doesn't set it) and Reinforce not called.

- [ ] **Step 8.3: Implement the orchestrator changes**

In `internal/memory/orchestrator.go`:

(a) In `Ingest`, after chunking + embedding, set scope on each chunk before the Store call. Read `Metadata["scope"]`, default `"global"`:

```go
	scope := "global"
	if raw, ok := doc.Metadata["scope"].(string); ok && raw != "" {
		scope = raw
	}
	for i := range chunks {
		chunks[i].Scope = scope
	}
```

Place this right before the `if old, ok := doc.Metadata["supersedes"]...` branch so both the supersede and plain-upsert paths carry the scope.

(b) In `Answer`, after `Retriever.Retrieve` returns `candidates` and before `Fusion`, reinforce:

```go
	if len(candidates) > 0 {
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		if err := o.Store.Reinforce(ctx, ids); err != nil {
			log.Warn().Err(err).Msg("reinforce failed; serving answer anyway")
		}
	}
```

Add the zerolog import: `"github.com/rs/zerolog/log"`. (Confirm the orchestrator doesn't already import it; if `Store` is nil in any existing test path, guard with `if o.Store != nil`. The Answer-path tests always set Store, but the existing `TestOrchestrator_Answer_AsOf*` tests may use a fake store — they already provide one, so this is safe. If any Answer test leaves Store nil, add the `o.Store != nil` guard.)

- [ ] **Step 8.4: Run to verify pass**

Run: `go test ./internal/memory/ -run TestOrchestrator -v`
Expected: PASS (new tests + all existing orchestrator tests).

- [ ] **Step 8.5: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/orchestrator_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Orchestrator tags ingest scope + reinforces on read

Ingest reads Metadata["scope"] (default 'global') and tags every chunk;
mirrors the Metadata["supersedes"] pattern. Answer reinforces the retrieved
ids after Retrieve and before Fusion (one batched UPDATE); a reinforce
failure is logged but never fails the query.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Sweeper — sweepOnce + archiveDecayed + purgeDead

**Files:**
- Create: `internal/memory/sweep.go`
- Test: `internal/memory/sweep_test.go`

- [ ] **Step 9.1: Write the failing integration tests**

Create `internal/memory/sweep_test.go`:

```go
package memory

import (
	"context"
	"testing"
	"time"
)

func gcTestConfig() GCConfig {
	return GCConfig{
		SweepEnabled:     true,
		SweepInterval:    time.Hour,
		DecayLambda:      0.05,
		ArchiveThreshold: 0.1,
		PurgeEnabled:     false,
		PurgeGraceDays:   30,
	}
}

func TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	old := time.Now().UTC().Add(-365 * 24 * time.Hour) // very old, unaccessed
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "proj", Content: "p", Embedding: []float32{1, 0, 0, 0}, PublishedAt: old},
		{ID: "glob", Content: "g", Embedding: []float32{1, 0, 0, 0}, PublishedAt: old},
	}); err != nil {
		t.Fatal(err)
	}
	// proj → project, both last_accessed long ago, access_count 0.
	if _, err := conn.Exec(context.Background(), `
		UPDATE chunks SET scope='project', last_accessed_at=$1 WHERE id='proj'`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `
		UPDATE chunks SET last_accessed_at=$1 WHERE id='glob'`, old); err != nil {
		t.Fatal(err)
	}

	sw := &Sweeper{conn: conn, cfg: gcTestConfig()}
	if err := sw.sweepOnce(context.Background()); err != nil {
		t.Fatalf("sweepOnce: %v", err)
	}

	var projStatus, globStatus string
	_ = conn.QueryRow(context.Background(), `SELECT status FROM chunks WHERE id='proj'`).Scan(&projStatus)
	_ = conn.QueryRow(context.Background(), `SELECT status FROM chunks WHERE id='glob'`).Scan(&globStatus)
	if projStatus != "archived" {
		t.Fatalf("project row should be archived, got %q", projStatus)
	}
	if globStatus != "active" {
		t.Fatalf("global row must be UNTOUCHED, got %q", globStatus)
	}
	// Audit row for the archive.
	var auditCount int
	_ = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM chunk_audit WHERE chunk_id='proj' AND new_status='archived' AND reason='decay'`,
	).Scan(&auditCount)
	if auditCount != 1 {
		t.Fatalf("expected 1 archive audit row for proj, got %d", auditCount)
	}
	// Global row has NO audit entry.
	var globAudit int
	_ = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM chunk_audit WHERE chunk_id='glob'`).Scan(&globAudit)
	if globAudit != 0 {
		t.Fatalf("global row must not be audited, got %d", globAudit)
	}
}

func TestSweeper_ArchiveIsReversible(t *testing.T) {
	conn := freshDB(t)
	_, err := conn.Exec(context.Background(), `
		INSERT INTO chunks (id, content, embedding, metadata, published_at, scope, status, archived_at)
		VALUES ('a', 'x', NULL, '{}', now(), 'project', 'archived', now())`)
	if err != nil {
		t.Fatal(err)
	}
	// One UPDATE restores it.
	if _, err := conn.Exec(context.Background(),
		`UPDATE chunks SET status='active', archived_at=NULL WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = conn.QueryRow(context.Background(), `SELECT status FROM chunks WHERE id='a'`).Scan(&status)
	if status != "active" {
		t.Fatalf("archive should be reversible by one UPDATE, got %q", status)
	}
}

func TestSweeper_PurgeDead_GatedAndGraced(t *testing.T) {
	conn := freshDB(t)
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-1 * 24 * time.Hour)
	mustInsertDead := func(id string, deadAt time.Time) {
		if _, err := conn.Exec(context.Background(), `
			INSERT INTO chunks (id, content, embedding, metadata, published_at, scope, status, dead_at)
			VALUES ($1, 'x', NULL, '{}', now(), 'project', 'dead', $2)`, id, deadAt); err != nil {
			t.Fatal(err)
		}
	}
	mustInsertDead("old_dead", old)
	mustInsertDead("recent_dead", recent)

	// Gate OFF: nothing deleted.
	swOff := &Sweeper{conn: conn, cfg: gcTestConfig()} // PurgeEnabled=false
	if err := swOff.sweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = conn.QueryRow(context.Background(), `SELECT count(*) FROM chunks WHERE status='dead'`).Scan(&n)
	if n != 2 {
		t.Fatalf("purge disabled: both dead rows should survive, got %d", n)
	}

	// Gate ON, 30-day grace: only old_dead (40d) is deleted, recent_dead (1d) survives.
	cfg := gcTestConfig()
	cfg.PurgeEnabled = true
	swOn := &Sweeper{conn: conn, cfg: cfg}
	if err := swOn.sweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var oldExists, recentExists bool
	_ = conn.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM chunks WHERE id='old_dead')`).Scan(&oldExists)
	_ = conn.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM chunks WHERE id='recent_dead')`).Scan(&recentExists)
	if oldExists {
		t.Fatalf("old_dead (40d) should be purged")
	}
	if !recentExists {
		t.Fatalf("recent_dead (1d) should survive the 30-day grace")
	}
	var purgeAudit int
	_ = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM chunk_audit WHERE chunk_id='old_dead' AND new_status='purged'`).Scan(&purgeAudit)
	if purgeAudit != 1 {
		t.Fatalf("expected purge audit row for old_dead, got %d", purgeAudit)
	}
}
```

- [ ] **Step 9.2: Run to verify failure**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run TestSweeper -v
```
Expected: FAIL — `undefined: Sweeper`.

- [ ] **Step 9.3: Implement the Sweeper**

Create `internal/memory/sweep.go`:

```go
package memory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Sweeper runs the GC's off-hot-path maintenance. Phase 4 passes: archiveDecayed
// (project rows below the decay threshold) and the gated purgeDead. Dedup and
// supersession resolution are Phase 5.
type Sweeper struct {
	conn *pgx.Conn
	cfg  GCConfig
}

func NewSweeper(conn *pgx.Conn, cfg GCConfig) *Sweeper {
	return &Sweeper{conn: conn, cfg: cfg}
}

// sweepOnce runs the Phase 4 passes in order. Directly callable in tests.
func (s *Sweeper) sweepOnce(ctx context.Context) error {
	if err := s.archiveDecayed(ctx); err != nil {
		return fmt.Errorf("archive pass: %w", err)
	}
	if err := s.purgeDead(ctx); err != nil {
		return fmt.Errorf("purge pass: %w", err)
	}
	return nil
}

// archiveDecayed archives project rows whose decay score is below the threshold.
// The scope='project' clause IS the firewall — global rows are structurally
// unreachable. Each transition is audited.
func (s *Sweeper) archiveDecayed(ctx context.Context) error {
	_, err := s.conn.Exec(ctx, `
		WITH moved AS (
		  UPDATE chunks SET status='archived', archived_at=now()
		   WHERE scope='project' AND status='active' AND NOT pinned
		     AND importance * exp(-$1 * extract(epoch FROM now()-last_accessed_at)/86400
		                          / (1 + access_count)) < $2
		  RETURNING id
		)
		INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
		SELECT id, 'active', 'archived', 'decay' FROM moved
	`, s.cfg.DecayLambda, s.cfg.ArchiveThreshold)
	return err
}

// purgeDead deletes rows that have been 'dead' for at least PurgeGraceDays.
// The ONLY destructive op; a no-op unless PurgeEnabled. Audited before delete,
// inside a transaction so audit + delete are atomic.
func (s *Sweeper) purgeDead(ctx context.Context) error {
	if !s.cfg.PurgeEnabled {
		return nil
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin purge tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
		SELECT id, 'dead', 'purged', 'purge' FROM chunks
		 WHERE status='dead' AND dead_at <= now() - make_interval(days => $1::int)
	`, int(s.cfg.PurgeGraceDays)); err != nil {
		return fmt.Errorf("purge audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM chunks
		 WHERE status='dead' AND dead_at <= now() - make_interval(days => $1::int)
	`, int(s.cfg.PurgeGraceDays)); err != nil {
		return fmt.Errorf("purge delete: %w", err)
	}
	return tx.Commit(ctx)
}
```

NOTE: `make_interval(days => $1::int)` is the safe parameterized way to build a day interval (avoids string concatenation). `PurgeGraceDays` is a float in config but purge granularity is whole days — `int()` truncation is fine (grace is a coarse floor).

- [ ] **Step 9.4: Run to verify pass**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run TestSweeper -v
```
Expected: PASS (3 tests).

- [ ] **Step 9.5: Commit**

```bash
git add internal/memory/sweep.go internal/memory/sweep_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Sweeper with archiveDecayed + gated purgeDead

sweepOnce runs the Phase 4 passes: archiveDecayed (scope='project' only —
the firewall; global rows structurally unreachable; each transition audited)
then purgeDead (the only destructive op, no-op unless GC_PURGE_ENABLED, deletes
rows dead >= grace days, audit+delete in one tx).

Firewall acceptance test: old project row archived, same-age global untouched
+ unaudited. Purge gating + 30-day grace covered.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Sweeper.Run (per-tick-tolerant loop)

**Files:**
- Modify: `internal/memory/sweep.go`
- Test: `internal/memory/sweep_test.go`

- [ ] **Step 10.1: Write the failing unit test (no DB)**

Append to `internal/memory/sweep_test.go`:

```go
func TestRunLoop_ContinuesAfterTickError(t *testing.T) {
	calls := 0
	tick := func(_ context.Context) error {
		calls++
		if calls < 3 {
			return context.DeadlineExceeded // simulate a failing tick
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runLoop(ctx, time.Millisecond, tick)
		close(done)
	}()
	// Let several ticks fire, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runLoop did not return after cancel")
	}
	if calls < 3 {
		t.Fatalf("loop should have continued past failing ticks, got %d calls", calls)
	}
}

func TestRunLoop_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	returned := make(chan struct{})
	go func() {
		runLoop(ctx, time.Millisecond, func(_ context.Context) error { return nil })
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("runLoop should return promptly on a cancelled ctx")
	}
}
```

- [ ] **Step 10.2: Run to verify failure**

Run: `go test ./internal/memory/ -run TestRunLoop -v`
Expected: FAIL — `undefined: runLoop`.

- [ ] **Step 10.3: Implement runLoop + Run**

Append to `internal/memory/sweep.go` (add `"time"` and `"github.com/rs/zerolog/log"` to imports):

```go
// Run ticks every cfg.SweepInterval and runs a sweep. Per-tick-tolerant: a
// failed sweep is logged at Error and the loop continues to the next interval.
// Returns when ctx is cancelled.
func (s *Sweeper) Run(ctx context.Context) {
	log.Info().Dur("interval", s.cfg.SweepInterval).Msg("memory sweep loop started")
	runLoop(ctx, s.cfg.SweepInterval, s.sweepOnce)
	log.Info().Msg("memory sweep loop stopped")
}

// runLoop is the testable core: tick on the interval, tolerate per-tick errors,
// stop on ctx cancellation. Extracted so the loop can be unit-tested without a DB.
func runLoop(ctx context.Context, interval time.Duration, tick func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := tick(ctx); err != nil {
				log.Error().Err(err).Msg("memory sweep tick failed; retrying next interval")
			}
		}
	}
}
```

- [ ] **Step 10.4: Run to verify pass**

Run: `go test ./internal/memory/ -run 'TestRunLoop|TestSweeper' -v`
Expected: PASS (the 2 loop unit tests run without a DB; the Sweeper integration tests need `DATABASE_URL`).

- [ ] **Step 10.5: Commit**

```bash
git add internal/memory/sweep.go internal/memory/sweep_test.go
git commit -m "$(cat <<'EOF'
feat(memory): Sweeper.Run per-tick-tolerant ticker loop

Run ticks every GC_SWEEP_INTERVAL and calls sweepOnce; a failed tick logs at
Error and the loop continues (the goroutine never dies). Stops on ctx cancel.
The loop core (runLoop) is extracted for DB-free unit testing of tolerance +
prompt cancellation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: MaybeStartSweep + workerd wiring

**Files:**
- Modify: `internal/memory/module.go`
- Test: `internal/memory/module_test.go`
- Modify: `cmd/workerd/main.go`

- [ ] **Step 11.1: Write the failing disabled-path test**

Append to `internal/memory/module_test.go`:

```go
func TestMaybeStartSweep_DisabledWhenUnset(t *testing.T) {
	// No GC_SWEEP_ENABLED → disabled, regardless of DATABASE_URL.
	t.Setenv("GC_SWEEP_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	cleanup, enabled, err := MaybeStartSweep(context.Background())
	if err != nil {
		t.Fatalf("disabled path should not error, got %v", err)
	}
	if enabled {
		t.Fatal("expected enabled=false")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup when disabled")
	}
}
```

- [ ] **Step 11.2: Run to verify failure**

Run: `go test ./internal/memory/ -run TestMaybeStartSweep_DisabledWhenUnset -v`
Expected: FAIL — `undefined: MaybeStartSweep`.

- [ ] **Step 11.3: Implement MaybeStartSweep**

Append to `internal/memory/module.go` (imports needed: `context`, `os`, `strings`, `github.com/jackc/pgx/v5` — most already present; add what's missing):

```go
// MaybeStartSweep loads MemoryConfig and, if GC_SWEEP_ENABLED is true AND a
// DATABASE_URL is configured, opens a pgx connection, applies migrations, and
// starts the sweep goroutine bound to ctx. Returns a cleanup func (closes the
// connection — call it on shutdown AFTER cancelling ctx) and an enabled flag.
//
// Never panics. A memory-side failure (bad config, unreachable DB) returns
// (nil, true, err) so the caller can log loudly and continue serving its core
// duties without the sweep.
func MaybeStartSweep(ctx context.Context) (cleanup func(), enabled bool, err error) {
	if !boolEnv("GC_SWEEP_ENABLED", false) {
		return nil, false, nil
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		return nil, false, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, true, fmt.Errorf("memory config: %w", err)
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, true, fmt.Errorf("connect postgres: %w", err)
	}
	if err := ApplyMigrations(ctx, conn, cfg.EmbeddingDim); err != nil {
		_ = conn.Close(ctx)
		return nil, true, fmt.Errorf("migrate: %w", err)
	}
	sw := NewSweeper(conn, cfg.GC)
	go sw.Run(ctx)
	return func() { _ = conn.Close(context.Background()) }, true, nil
}
```

(Add `"os"` and `"strings"` to module.go imports if absent.)

- [ ] **Step 11.4: Run to verify pass**

Run: `go test ./internal/memory/ -run TestMaybeStartSweep -v`
Expected: PASS.

- [ ] **Step 11.5: Wire workerd**

In `cmd/workerd/main.go`:

(a) Add the import `"github.com/luannn010/ptolemy/internal/memory"`.

(b) After the existing wiring (just before the `go func()` server starts, or right after them — place it before the `<-stop` block), add:

```go
	sweepCtx, cancelSweep := context.WithCancel(context.Background())
	sweepCleanup, sweepEnabled, sweepErr := memory.MaybeStartSweep(sweepCtx)
	switch {
	case sweepErr != nil:
		log.Error().Err(sweepErr).Msg("memory sweep enabled but failed to start; continuing without it")
	case sweepEnabled:
		log.Info().Msg("memory sweep started")
	default:
		log.Info().Msg("memory sweep disabled (set DATABASE_URL and GC_SWEEP_ENABLED=true to enable)")
	}
```

(c) In the shutdown sequence (after `<-stop`), cancel + clean up before/around the server shutdown:

```go
	<-stop

	cancelSweep()
	if sweepCleanup != nil {
		sweepCleanup()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = approveServer.Shutdown(ctx)
```

- [ ] **Step 11.6: Verify workerd builds + the no-Postgres path**

Run: `go build ./cmd/workerd && go vet ./cmd/workerd`
Expected: success.

Manually confirm the disabled path doesn't require Postgres:
```bash
GC_SWEEP_ENABLED=false ./bin/workerd  # should log "memory sweep disabled" and serve; Ctrl-C to stop
```
(Build first with `make build`. This is a manual smoke; no automated assertion.)

- [ ] **Step 11.7: Commit**

```bash
git add internal/memory/module.go internal/memory/module_test.go cmd/workerd/main.go
git commit -m "$(cat <<'EOF'
feat(memory): MaybeStartSweep + opt-in workerd goroutine

MaybeStartSweep loads config and, when GC_SWEEP_ENABLED + DATABASE_URL are
set, opens a conn, migrates, and starts the sweep bound to ctx; returns a
cleanup func. Never panics — enabled-but-failed returns (nil,true,err) so
workerd logs loudly and keeps serving.

workerd calls it after its existing wiring and distinguishes disabled (Info)
from enabled-but-failed (Error); cancels the sweep ctx + closes the conn on
shutdown. Existing Postgres-free workerd deployments are unaffected.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Commit GC reference drafts

**Files:**
- Track: the six untracked `docs/memory/*-GC.md` / `PHASE_*.md` drafts.

- [ ] **Step 12.1: Stage + commit the drafts as historical reference**

```bash
git add docs/memory/PHASE_4_MEMORY_GC_CORE.md \
        docs/memory/PHASE_5_MEMORY_GC_SUPERSESSION_DEDUP.md \
        docs/memory/PHASE_6_CONVERSATIONAL_MEMORY.md \
        docs/memory/README-GC.md \
        docs/memory/SPEC-GC.md \
        docs/memory/TASKS-GC.md
git commit -m "$(cat <<'EOF'
docs(memory): track the GC source drafts as historical reference

The Memory-GC package drafts (bridge + generic SPEC/TASKS/README + Phase 5/6
forward notes) that seeded Phase 4. The Phase 4 spec at
docs/superpowers/specs/2026-05-28-memory-phase4.md is the authoritative
Ptolemy-adapted design where the two differ (chunks/chunk_audit not
memories/memory_audit; ParadeDB not ts_rank_cd; decay-blend deferred).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Docs — .env.example + DATA_MODEL + IMPLEMENTATION_PLAN

**Files:**
- Modify: `.env.example`, `docs/memory/DATA_MODEL.md`, `docs/memory/IMPLEMENTATION_PLAN.md`

- [ ] **Step 13.1: .env.example — add the GC vars**

Append to `.env.example`:

```
# Phase 4 Memory GC. Defaults are PLACEHOLDERS — tune against real chat volume
# via Stats() once Phase 6 produces project rows. The sweep is opt-in and only
# runs in workerd when GC_SWEEP_ENABLED=true AND DATABASE_URL is set.
GC_SWEEP_ENABLED=false
GC_SWEEP_INTERVAL=1h
GC_DECAY_LAMBDA=0.05
GC_ARCHIVE_THRESHOLD=0.1
GC_PURGE_ENABLED=false
GC_PURGE_GRACE_DAYS=30
```

- [ ] **Step 13.2: DATA_MODEL.md — document the lifecycle columns**

In `docs/memory/DATA_MODEL.md`, under the `chunks` table section, add a "Phase 4 (GC lifecycle)" subsection listing the columns (`scope`, `status`, `importance`, `pinned`, `access_count`, `last_accessed_at`, `confidence`, `version`, `supersedes`, `archived_at`, `dead_at`, `fact_subject`, `fact_predicate`) with one-line notes, and a short `chunk_audit` table description. Keep it consistent with the existing column-notes style. Note that `global` immunity is schema-enforced (the archive query's `scope='project'` clause).

- [ ] **Step 13.3: IMPLEMENTATION_PLAN.md — add a Phase 4 section**

In `docs/memory/IMPLEMENTATION_PLAN.md`, after the Phase 3 section, add:

```markdown
## Phase 4 — Memory GC Core

Status lifecycle on `chunks` + reinforcement + observability + a dormant sweep.
Built and tested now; the decay/archive passes target `scope='project'` rows,
which don't exist until Phase 6.

- [x] Migration `0004_chunks_gc_lifecycle`: full Phase 4-6 column set + `chunk_audit` + 2 indexes.
- [x] `decayScore()` + Go-vs-SQL agreement test.
- [x] `Store.Reinforce` (bump on read) + `Store.Stats` (counts by scope×status).
- [x] Retrieval filters `status='active'` (firewall read half). Decay-ranking-blend DEFERRED to Phase 6.
- [x] `Orchestrator`: ingest tags `scope` (default global); `Answer` reinforces post-retrieve.
- [x] `Sweeper`: per-tick-tolerant `Run`; `archiveDecayed` (project-only firewall, audited) + gated `purgeDead` (dead_at-anchored grace).
- [x] `MaybeStartSweep` + opt-in workerd goroutine (gated by DATABASE_URL + GC_SWEEP_ENABLED; log-and-continue).
- [x] GC config knobs (lambda, threshold, interval, purge grace, both enables) — placeholders to tune via Stats().

**Acceptance:**
- [x] Synthetic old `scope='project'` row archived by the sweep; same-age `global` row untouched + unaudited (`TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched`).
- [x] Hybrid retrieval preserved; archive reversible by one UPDATE; every transition audited.
- [x] No regression: `make eval-memory` still recall@5 = 1.000 over 30 questions after the migration.

**Deferred:** dedup + supersession resolution (Phase 5); conversational capture + decay-ranking-blend (Phase 6); archived→dead aging (purge tested with a synthetic dead row).
```

- [ ] **Step 13.4: Commit**

```bash
git add .env.example docs/memory/DATA_MODEL.md docs/memory/IMPLEMENTATION_PLAN.md
git commit -m "$(cat <<'EOF'
docs(memory): document Phase 4 GC env vars, schema, and roadmap entry

.env.example: the 6 GC knobs (placeholder note). DATA_MODEL.md: lifecycle
columns + chunk_audit + schema-enforced global immunity. IMPLEMENTATION_PLAN.md:
Phase 4 section with file/test pointers + acceptance + deferrals.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Final verification

- [ ] **Step 14.1: Full unit suite (no DB)**

Run: `make test`
Expected: PASS across all packages (integration tests skip cleanly without `DATABASE_URL`).

- [ ] **Step 14.2: Integration suite (live DB)**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test -p 1 ./internal/memory/...
```
Expected: PASS (migration, Reinforce, Stats, decay agreement, status filter, sweeper firewall/purge).

- [ ] **Step 14.3: No-regression eval (REQUIRES DB RESET — user-authorized)**

```bash
PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy \
  -c 'DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE;'
make eval-memory 2>&1 | tail -10
```
Expected: `mean recall@5 = 1.000 over 30 questions (fixture_version=1)`. The `status='active'` filter drops nothing (all ingested rows are active). **This is the acceptance no-regression gate.**

- [ ] **Step 14.4: Smoke (reinforcement on read path)**

```bash
PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy \
  -c 'DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE;'
make smoke-memory 2>&1 | tail -15
```
Expected: a grounded answer with citations; no reinforce error in the logs.

- [ ] **Step 14.5: Coverage gate (with DB)**

Run:
```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test -p 1 -cover ./internal/memory/...
```
Expected: `internal/memory` ≥ 80%.

- [ ] **Step 14.6: Branch readiness**

Run: `git log --oneline main..HEAD`
Expected: the spec commit + ~13 task commits, all with the `Co-Authored-By` trailer. Working tree clean.

- [ ] **Step 14.7: PR preparation**

Do NOT use `gh` (AGENTS.md). Surface a PR-description draft for web-UI submission:

```markdown
## Summary
Phase 4 — Memory GC Core. Status lifecycle on `chunks` (migration 0004 +
chunk_audit), reinforcement-on-read, Stats observability, status='active'
retrieval filter, and an opt-in workerd-launched background sweep
(archiveDecayed + gated purgeDead).

## Acceptance
- Firewall: synthetic old project row archived; same-age global row untouched + unaudited.
- No regression: `make eval-memory` recall@5 = 1.000 over 30 questions after the migration.
- Sweep is opt-in (GC_SWEEP_ENABLED + DATABASE_URL); workerd unaffected when disabled.

## Deferred
- Dedup + supersession resolution (Phase 5); conversational capture + decay-ranking-blend (Phase 6).
- The decay/archive passes are dormant until Phase 6 produces project rows.
- Tuning defaults (lambda, threshold, grace) are placeholders — tune via Stats().

## Test plan
- [x] `make test`
- [x] `DATABASE_URL=... go test -p 1 ./internal/memory/...`
- [x] `make eval-memory` → recall@5 = 1.000
- [x] `make smoke-memory` → grounded answer
- [x] coverage ≥ 80% on internal/memory

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## Out of scope (NOT in this plan)

- Dedup pass (pg_trgm) + supersession resolution + structured-fact ladder + `confidence` news-flow — Phase 5.
- Conversational/project capture + decay-ranking-blend — Phase 6.
- `archived→dead` aging transition — no Phase 4 pass produces `dead` rows; purge tested with a synthetic dead row.
- Reranker, query expansion, topic_digests — still-deferred Phase 3 leftovers.

---

## Self-review

Checked against `docs/superpowers/specs/2026-05-28-memory-phase4.md`:

- **Spec coverage:** migration (T1), Chunk fields (T2), decayScore (T3), GCConfig (T4), Reinforce/Stats (T5), Upsert scope + Go-vs-SQL (T6), status filter (T7), orchestrator ingest-scope + reinforce (T8), Sweeper passes (T9), Run loop (T10), MaybeStartSweep + workerd (T11), GC drafts commit (T12), docs (T13), verification (T14). Every spec component maps to a task.
- **Placeholder scan:** no TBD/TODO/"handle errors" — every code step has complete code. The one judgment note (fake-store receiver name in T5.4) instructs a `grep` to confirm the actual name, not a placeholder.
- **Type consistency:** `GCConfig` fields, `ScopeStatusCount`, `Sweeper{conn,cfg}`, `decayScore(importance, scope, pinned, accessCount, daysSinceAccess, lambda)`, `MaybeStartSweep(ctx)(cleanup,enabled,err)`, `runLoop(ctx,interval,tick)` — names consistent across the tasks that reference them.
- **Spec-vs-plan corrections folded in:** the spec said interval floor 1m, but `TestLoadConfig_GCEnvParsed` + the manual workerd smoke use `5s`; the plan resolves this to a **1s** floor (documented in T4.1) so dev/test fast intervals are legal with no correctness hazard.
