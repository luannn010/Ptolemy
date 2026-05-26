# Memory Module — Phase 0 (Vector MVP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the end-to-end RAG-style memory pipeline described in `docs/memory/`: ingest text → chunk → embed (OpenAI-compatible HTTP) → store in Postgres+pgvector → vector retrieve → context build → LLM generate (with citations). Quality tuning, hybrid search, freshness, and reranking are explicitly out of scope per the spec's phase-order rule.

**Architecture:** New Go package `internal/memory` containing one struct per spec interface (`Embedder`, `Chunker`, `Store`, `Retriever`, `Fusion`, `ContextBuilder`, `Generator`) and an `Orchestrator` that wires them from a `MemoryConfig`. Every dependency on Postgres or external HTTP services goes through an interface, so tests use httptest + a fake store, and only a single integration test touches real Postgres (skipped when `MEMORY_DATABASE_URL` is unset).

**Tech Stack:** Go 1.25, `github.com/jackc/pgx/v5` (Postgres driver), `github.com/pgvector/pgvector-go` (vector type adapter), existing `internal/config` + `internal/logging`. Env vars (reusing .env.example names where they exist):
- Reused from `.env.example`: `DATABASE_URL`, `BRAIN_BASE_URL`, `BRAIN_MODEL`, `RAG_TOP_K`, `RAG_CHUNK_SIZE_TOKENS`, `RAG_CHUNK_OVERLAP_TOKENS`.
- New: `EMBEDDING_BASE_URL`, `EMBEDDING_MODEL`, `EMBEDDING_DIM`, `EMBEDDING_API_KEY`.

**Spec:** [docs/memory/](../../memory/) — README, ARCHITECTURE, DATA_MODEL, RETRIEVAL, IMPLEMENTATION_PLAN

**Locked decisions (from brainstorming):**
- Branch: `ptolemy/memory-phase0` (already created)
- Phase 0 only — no hybrid, freshness, or reranker in this PR
- Embeddings + LLM use OpenAI-compatible `/v1/embeddings` and `/v1/chat/completions`
- Single-tenant (`tenant_id` column nullable, never filtered)
- BM25 backend deferred to Phase 1

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/memory/config.go` | Create | `MemoryConfig` loaded from env; validation; embedding dimension as int |
| `internal/memory/types.go` | Create | `Chunk`, `RetrievedChunk`, `Query`, `RawDocument`, `ParsedDocument`, `PromptContext`, `Answer` |
| `internal/memory/migrations/0001_chunks_core.sql` | Create | `CREATE EXTENSION vector` + `chunks` table + HNSW index (Phase 0 columns only) |
| `internal/memory/migrations.go` | Create | Apply embedded `.sql` files to a `*pgx.Conn` in order |
| `internal/memory/store.go` | Create | `Store` interface + `PgStore` (Upsert / Get / MarkSuperseded) backed by pgx + pgvector |
| `internal/memory/embedder.go` | Create | `Embedder` interface + `OpenAIEmbedder` (POST `/v1/embeddings`, batched) |
| `internal/memory/chunker.go` | Create | `Chunker` interface + `FixedSizeChunker(maxRunes, overlap)` |
| `internal/memory/retriever.go` | Create | `Retriever` interface + `VectorRetriever` (single CTE — no BM25 yet) |
| `internal/memory/fusion.go` | Create | `Fusion` interface + `PassthroughFusion` |
| `internal/memory/context_builder.go` | Create | `ContextBuilder` interface + `BudgetContextBuilder(tokenBudget int)` |
| `internal/memory/generator.go` | Create | `Generator` interface + `OpenAIGenerator` (POST `/v1/chat/completions`) |
| `internal/memory/orchestrator.go` | Create | `Orchestrator.Ingest(ctx, RawDocument)` and `Orchestrator.Answer(ctx, Query)` |
| `internal/memory/module.go` | Create | `NewModule(MemoryConfig)` factory wiring all components, returns `*Orchestrator` |
| `cmd/memory-demo/main.go` | Create | CLI: `memory-demo ingest <file>` and `memory-demo ask "<question>"` for manual end-to-end exercising |
| `internal/config/config.go` | Modify | Add 8 memory-related env getters (no defaults that hide misconfiguration) |
| `go.mod` / `go.sum` | Modify | Add `github.com/jackc/pgx/v5` and `github.com/pgvector/pgvector-go` |
| `Makefile` | Modify | Add `memory-demo` to the `build` target |

Each test file lives next to the source file with the same prefix (`store_test.go`, `embedder_test.go`, etc.).

**Coverage note:** the existing CI runs `go test -coverpkg=./internal/...`, so `internal/memory/*` is in scope for the 80% gate. Every component must have tests in this plan.

---

## Task 1: Add Postgres + pgvector dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add dependencies**

Run:

```bash
go get github.com/jackc/pgx/v5@latest
go get github.com/pgvector/pgvector-go@latest
```

- [ ] **Step 2: Verify the module still builds**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build(memory): add pgx v5 + pgvector-go for memory module"
```

---

## Task 2: Memory config from env

**Files:**
- Create: `internal/memory/config.go`
- Create: `internal/memory/config_test.go`
- Modify: `internal/config/config.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/config_test.go`:

```go
package memory

import (
	"testing"
)

func TestLoadConfig_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected DATABASE_URL to be required")
	}
}

func TestLoadConfig_ParsesAllFields(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/d?sslmode=disable")
	t.Setenv("EMBEDDING_BASE_URL", "http://embed:8000")
	t.Setenv("EMBEDDING_MODEL", "bge-large")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("EMBEDDING_API_KEY", "ek")
	t.Setenv("BRAIN_BASE_URL", "http://llm:8000")
	t.Setenv("BRAIN_MODEL", "qwen")
	t.Setenv("RAG_TOP_K", "8")
	t.Setenv("RAG_CHUNK_SIZE_TOKENS", "700")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "100")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://u:p@h:5432/d?sslmode=disable" {
		t.Fatalf("DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.EmbeddingDim != 1024 {
		t.Fatalf("EmbeddingDim: %d", cfg.EmbeddingDim)
	}
	if cfg.EmbeddingBaseURL != "http://embed:8000" || cfg.EmbeddingModel != "bge-large" || cfg.EmbeddingAPIKey != "ek" {
		t.Fatalf("embedding fields wrong: %+v", cfg)
	}
	if cfg.LLMBaseURL != "http://llm:8000" || cfg.LLMModel != "qwen" {
		t.Fatalf("llm fields wrong: %+v", cfg)
	}
	if cfg.TopK != 8 || cfg.ChunkSizeTokens != 700 || cfg.ChunkOverlapTokens != 100 {
		t.Fatalf("RAG knobs wrong: %+v", cfg)
	}
}

func TestLoadConfig_RejectsZeroEmbeddingDim(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/d")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "0")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected EMBEDDING_DIM=0 to be rejected")
	}
}

func TestLoadConfig_RAGKnobsDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_TOP_K", "")
	t.Setenv("RAG_CHUNK_SIZE_TOKENS", "")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TopK != 8 || cfg.ChunkSizeTokens != 700 || cfg.ChunkOverlapTokens != 100 {
		t.Fatalf("expected defaults 8/700/100, got %d/%d/%d", cfg.TopK, cfg.ChunkSizeTokens, cfg.ChunkOverlapTokens)
	}
}
```

- [ ] **Step 2: Run; expect compile failure**

Run: `go test ./internal/memory/...`
Expected: `LoadConfig undefined`.

- [ ] **Step 3: Implement config**

Create `internal/memory/config.go`:

```go
package memory

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MemoryConfig is loaded entirely from environment variables, reusing the
// names already defined in .env.example. There are no defaults for service
// endpoints (DATABASE_URL, EMBEDDING_*, BRAIN_*): a missing endpoint is a
// misconfiguration that should fail fast at startup, not silently fall back.
// The RAG knobs (TopK, chunk sizes) have sensible defaults matching .env.example.
type MemoryConfig struct {
	DatabaseURL string

	EmbeddingBaseURL string
	EmbeddingModel   string
	EmbeddingDim     int
	EmbeddingAPIKey  string

	// LLM endpoint reuses the existing BRAIN_BASE_URL / BRAIN_MODEL env vars.
	LLMBaseURL string
	LLMModel   string

	TopK               int
	ChunkSizeTokens    int
	ChunkOverlapTokens int
}

func LoadConfig() (MemoryConfig, error) {
	cfg := MemoryConfig{
		DatabaseURL:      strings.TrimSpace(os.Getenv("DATABASE_URL")),
		EmbeddingBaseURL: strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")),
		EmbeddingModel:   strings.TrimSpace(os.Getenv("EMBEDDING_MODEL")),
		EmbeddingAPIKey:  strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")),
		LLMBaseURL:       strings.TrimSpace(os.Getenv("BRAIN_BASE_URL")),
		LLMModel:         strings.TrimSpace(os.Getenv("BRAIN_MODEL")),
	}

	if cfg.DatabaseURL == "" {
		return MemoryConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.EmbeddingBaseURL == "" || cfg.EmbeddingModel == "" {
		return MemoryConfig{}, fmt.Errorf("EMBEDDING_BASE_URL and EMBEDDING_MODEL are required")
	}
	if cfg.LLMBaseURL == "" || cfg.LLMModel == "" {
		return MemoryConfig{}, fmt.Errorf("BRAIN_BASE_URL and BRAIN_MODEL are required")
	}

	dimStr := strings.TrimSpace(os.Getenv("EMBEDDING_DIM"))
	dim, err := strconv.Atoi(dimStr)
	if err != nil || dim <= 0 {
		return MemoryConfig{}, fmt.Errorf("EMBEDDING_DIM must be a positive integer, got %q", dimStr)
	}
	cfg.EmbeddingDim = dim

	cfg.TopK = intEnv("RAG_TOP_K", 8)
	cfg.ChunkSizeTokens = intEnv("RAG_CHUNK_SIZE_TOKENS", 700)
	cfg.ChunkOverlapTokens = intEnv("RAG_CHUNK_OVERLAP_TOKENS", 100)

	return cfg, nil
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/memory/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go
git commit -m "feat(memory): MemoryConfig from env (reuses .env.example names)

Reuses DATABASE_URL, BRAIN_BASE_URL, BRAIN_MODEL, RAG_TOP_K,
RAG_CHUNK_SIZE_TOKENS, RAG_CHUNK_OVERLAP_TOKENS from .env.example;
adds EMBEDDING_BASE_URL, EMBEDDING_MODEL, EMBEDDING_DIM,
EMBEDDING_API_KEY for the hosted embedding model. No silent
defaults for endpoints — missing values fail fast at startup."
```

---

## Task 3: Core types

**Files:**
- Create: `internal/memory/types.go`

- [ ] **Step 1: Add the types**

Create `internal/memory/types.go`:

```go
package memory

import "time"

// Chunk is the unit of retrieval. Phase 0 uses Content, Embedding, Metadata,
// Source, and Tenant; the freshness fields are present so Phase 2 can add
// behavior without a schema migration ordering nightmare, but they are
// nullable and unused this phase.
type Chunk struct {
	ID           string
	Content      string
	Embedding    []float32
	Metadata     map[string]any
	Source       string
	Tenant       string // empty in single-tenant deployments
	PublishedAt  time.Time
	ValidFrom    *time.Time
	ValidTo      *time.Time
	SupersededBy *string
	CreatedAt    time.Time
}

type RetrievedChunk struct {
	Chunk
	Score float64
}

type Query struct {
	Text    string
	K       int
	AsOf    *time.Time
	Filters map[string]any
}

type RawDocument struct {
	ID       string
	Source   string
	Text     string
	Metadata map[string]any
}

type ParsedDocument struct {
	ID          string
	Source      string
	Text        string
	Metadata    map[string]any
	PublishedAt time.Time
}

type PromptContext struct {
	System   string
	User     string
	SourceIDs []string
}

type Answer struct {
	Text      string
	Citations []string
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/memory/...`
Expected: clean build (no tests required — pure types).

- [ ] **Step 3: Commit**

```bash
git add internal/memory/types.go
git commit -m "feat(memory): core types (Chunk, Query, PromptContext, Answer)

Phase 0 only exercises Content/Embedding/Metadata/Source. The
freshness fields (PublishedAt, ValidFrom/To, SupersededBy) are
defined now so Phase 2 is purely additive in the SQL layer."
```

---

## Task 4: Migration runner + 0001_chunks_core

**Files:**
- Create: `internal/memory/migrations/0001_chunks_core.sql`
- Create: `internal/memory/migrations.go`
- Create: `internal/memory/migrations_test.go`

- [ ] **Step 1: Write the migration SQL**

Create `internal/memory/migrations/0001_chunks_core.sql`:

```sql
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
```

The `__EMBEDDING_DIM__` token is replaced at runtime so the vector dimension matches `MemoryConfig.EmbeddingDim`.

- [ ] **Step 2: Write a failing migration test**

Create `internal/memory/migrations_test.go`:

```go
package memory

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func requirePG(t *testing.T) string {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("MEMORY_DATABASE_URL"))
	if url == "" {
		t.Skip("MEMORY_DATABASE_URL not set; skipping Postgres integration test")
	}
	return url
}

func TestApplyMigrations_CreatesChunksTable(t *testing.T) {
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	// Clean slate for the test run.
	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE`)

	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var n int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.columns WHERE table_name='chunks' AND column_name='embedding'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("embedding column not created (count=%d)", n)
	}

	// Idempotency: second apply must succeed without error.
	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("second ApplyMigrations: %v", err)
	}
}
```

- [ ] **Step 3: Run; expect compile failure or skip**

Run: `go test ./internal/memory/... -run Migrations`
Expected: either `ApplyMigrations undefined` (compile error) or skip if no Postgres.

- [ ] **Step 4: Implement the migration runner**

Create `internal/memory/migrations.go`:

```go
package memory

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ApplyMigrations runs every embedded migration that hasn't been recorded in
// memory_schema_migrations. The dim parameter substitutes __EMBEDDING_DIM__ in
// the chunks_core migration; later migrations may ignore it.
func ApplyMigrations(ctx context.Context, conn *pgx.Conn, dim int) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS memory_schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("bootstrap migrations table: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists int
		if err := conn.QueryRow(ctx,
			`SELECT count(*) FROM memory_schema_migrations WHERE version = $1`, version,
		).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		data, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		sql := strings.ReplaceAll(string(data), "__EMBEDDING_DIM__", strconv.Itoa(dim))
		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := conn.Exec(ctx,
			`INSERT INTO memory_schema_migrations(version) VALUES ($1)`, version,
		); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Run integration test**

Run: `MEMORY_DATABASE_URL=postgres://... go test ./internal/memory/... -run Migrations`
Expected: PASS. If no Postgres available locally, expect SKIP — the test will run in CI once Postgres is added there.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/migrations.go internal/memory/migrations_test.go internal/memory/migrations/0001_chunks_core.sql
git commit -m "feat(memory): embedded SQL migrations runner + 0001_chunks_core

Embeds migrations/*.sql into the binary and runs un-applied ones in
filename order, tracked via memory_schema_migrations. __EMBEDDING_DIM__
is substituted from MemoryConfig.EmbeddingDim so the vector column
size matches the model. Idempotent; CI runs it on every startup."
```

---

## Task 5: PgStore (Store interface backed by pgx)

**Files:**
- Create: `internal/memory/store.go`
- Create: `internal/memory/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/store_test.go`:

```go
package memory

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func freshDB(t *testing.T) *pgx.Conn {
	t.Helper()
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE`)
	if err := ApplyMigrations(context.Background(), conn, 4); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

func TestPgStore_UpsertAndGet(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	chunks := []Chunk{{
		ID:          "c1",
		Content:     "hello world",
		Embedding:   []float32{1, 0, 0, 0},
		Metadata:    map[string]any{"k": "v"},
		Source:      "test",
		PublishedAt: time.Now().UTC(),
	}}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.Get(context.Background(), []string{"c1"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 || got[0].Content != "hello world" {
		t.Fatalf("unexpected get result: %+v", got)
	}
	if got[0].Metadata["k"] != "v" {
		t.Fatalf("metadata not round-tripped: %+v", got[0].Metadata)
	}
}

func TestPgStore_MarkSuperseded(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()
	chunks := []Chunk{
		{ID: "a", Content: "old", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now.Add(-time.Hour)},
		{ID: "b", Content: "new", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSuperseded(context.Background(), "a", "b"); err != nil {
		t.Fatalf("MarkSuperseded: %v", err)
	}
	got, _ := s.Get(context.Background(), []string{"a"})
	if got[0].SupersededBy == nil || *got[0].SupersededBy != "b" {
		t.Fatalf("expected SupersededBy=b, got %+v", got[0].SupersededBy)
	}
}
```

- [ ] **Step 2: Implement PgStore**

Create `internal/memory/store.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Store interface {
	Upsert(ctx context.Context, chunks []Chunk) error
	Get(ctx context.Context, ids []string) ([]Chunk, error)
	MarkSuperseded(ctx context.Context, oldID, newID string) error
}

type PgStore struct {
	conn *pgx.Conn
}

func NewPgStore(conn *pgx.Conn) *PgStore { return &PgStore{conn: conn} }

func (s *PgStore) Upsert(ctx context.Context, chunks []Chunk) error {
	for _, c := range chunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		_, err = s.conn.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				source = EXCLUDED.source,
				tenant_id = EXCLUDED.tenant_id,
				published_at = EXCLUDED.published_at
		`,
			c.ID, c.Content, pgvector.NewVector(c.Embedding), meta,
			nullableStr(c.Source), nullableStr(c.Tenant), c.PublishedAt,
		)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", c.ID, err)
		}
	}
	return nil
}

func (s *PgStore) Get(ctx context.Context, ids []string) ([]Chunk, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT id, content, embedding, metadata, COALESCE(source,''), COALESCE(tenant_id,''),
		       published_at, valid_from, valid_to, superseded_by, created_at
		FROM chunks WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var c Chunk
		var emb pgvector.Vector
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.Content, &emb, &metaJSON, &c.Source, &c.Tenant,
			&c.PublishedAt, &c.ValidFrom, &c.ValidTo, &c.SupersededBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Embedding = emb.Slice()
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &c.Metadata)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PgStore) MarkSuperseded(ctx context.Context, oldID, newID string) error {
	_, err := s.conn.Exec(ctx, `UPDATE chunks SET superseded_by = $1 WHERE id = $2`, newID, oldID)
	return err
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 3: Run integration tests**

Run: `MEMORY_DATABASE_URL=postgres://... go test ./internal/memory/... -run PgStore`
Expected: PASS. SKIP without Postgres.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/store.go internal/memory/store_test.go
git commit -m "feat(memory): PgStore — Upsert/Get/MarkSuperseded via pgx

Implements the spec's Store interface backed by pgx + pgvector-go.
Single-tenant: tenant_id stays nullable. Metadata is JSONB. Embedding
uses pgvector.Vector for proper binding. Tests skip if
MEMORY_DATABASE_URL is unset (local dev without Postgres still
builds + passes unit tests)."
```

---

## Task 6: FixedSizeChunker

**Files:**
- Create: `internal/memory/chunker.go`
- Create: `internal/memory/chunker_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/chunker_test.go`:

```go
package memory

import (
	"strings"
	"testing"
	"time"
)

func TestFixedSizeChunker_SplitsAndOverlaps(t *testing.T) {
	doc := ParsedDocument{
		ID:          "doc1",
		Text:        strings.Repeat("abcdefghij", 12), // 120 runes
		PublishedAt: time.Now(),
	}
	c := FixedSizeChunker{MaxRunes: 50, Overlap: 10}
	chunks := c.Chunk(doc)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if len([]rune(ch.Content)) == 0 || len([]rune(ch.Content)) > 50 {
			t.Fatalf("chunk size out of bounds: %d", len([]rune(ch.Content)))
		}
		if ch.ID == "" || !strings.HasPrefix(ch.ID, "doc1#") {
			t.Fatalf("unexpected chunk id %q", ch.ID)
		}
	}
}

func TestFixedSizeChunker_ShortDocSingleChunk(t *testing.T) {
	c := FixedSizeChunker{MaxRunes: 100, Overlap: 10}
	chunks := c.Chunk(ParsedDocument{ID: "d", Text: "tiny", PublishedAt: time.Now()})
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "tiny" {
		t.Fatalf("content: %q", chunks[0].Content)
	}
}

func TestFixedSizeChunker_RejectsBadConfig(t *testing.T) {
	c := FixedSizeChunker{MaxRunes: 0, Overlap: 0}
	chunks := c.Chunk(ParsedDocument{ID: "d", Text: "any", PublishedAt: time.Now()})
	if len(chunks) != 1 || chunks[0].Content != "any" {
		t.Fatalf("zero MaxRunes should yield a single passthrough chunk, got %+v", chunks)
	}
}
```

- [ ] **Step 2: Run; expect compile failure**

Run: `go test ./internal/memory/... -run Chunker`
Expected: `FixedSizeChunker undefined`.

- [ ] **Step 3: Implement chunker**

Create `internal/memory/chunker.go`:

```go
package memory

import (
	"fmt"
)

type Chunker interface {
	Chunk(doc ParsedDocument) []Chunk
}

// FixedSizeChunker splits text into rune-based windows with overlap. Rune
// arithmetic avoids byte mid-codepoint splits on multi-byte text. The unit is
// runes (not tokens) because tokenization is model-specific and Phase 0 must
// not bind us to a particular tokenizer.
type FixedSizeChunker struct {
	MaxRunes int
	Overlap  int
}

func (c FixedSizeChunker) Chunk(doc ParsedDocument) []Chunk {
	runes := []rune(doc.Text)
	if c.MaxRunes <= 0 || len(runes) <= c.MaxRunes {
		return []Chunk{{
			ID:          fmt.Sprintf("%s#0", doc.ID),
			Content:     doc.Text,
			Metadata:    doc.Metadata,
			Source:      doc.Source,
			PublishedAt: doc.PublishedAt,
		}}
	}
	step := c.MaxRunes - c.Overlap
	if step <= 0 {
		step = c.MaxRunes
	}
	var out []Chunk
	for start, i := 0, 0; start < len(runes); start, i = start+step, i+1 {
		end := start + c.MaxRunes
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, Chunk{
			ID:          fmt.Sprintf("%s#%d", doc.ID, i),
			Content:     string(runes[start:end]),
			Metadata:    doc.Metadata,
			Source:      doc.Source,
			PublishedAt: doc.PublishedAt,
		})
		if end == len(runes) {
			break
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/memory/... -run Chunker`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/chunker.go internal/memory/chunker_test.go
git commit -m "feat(memory): FixedSizeChunker with rune-based overlap

Rune (not byte, not token) windows avoid mid-codepoint splits and
keep Phase 0 free of a tokenizer dependency. Tokenization-aware
chunkers can land later behind the Chunker interface."
```

---

## Task 7: OpenAIEmbedder

**Files:**
- Create: `internal/memory/embedder.go`
- Create: `internal/memory/embedder_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/embedder_test.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbedder_BatchesAndUnpacks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/embeddings") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "test-model" || len(body.Input) != 2 {
			t.Errorf("unexpected body %+v", body)
		}
		resp := map[string]any{
			"data": []map[string]any{
				{"embedding": []float64{0.1, 0.2, 0.3}},
				{"embedding": []float64{0.4, 0.5, 0.6}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "test-model", "key-x")
	vecs, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 || vecs[0][0] != 0.1 || vecs[1][2] != 0.6 {
		t.Fatalf("unexpected vectors: %+v", vecs)
	}
}

func TestOpenAIEmbedder_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0]}]}`))
	}))
	defer srv.Close()
	e := NewOpenAIEmbedder(srv.URL, "m", "secret")
	if _, err := e.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization header: %q", gotAuth)
	}
}

func TestOpenAIEmbedder_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	e := NewOpenAIEmbedder(srv.URL, "m", "")
	if _, err := e.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatalf("expected error on 500")
	}
}
```

- [ ] **Step 2: Run; expect compile failure**

Run: `go test ./internal/memory/... -run Embedder`
Expected: `NewOpenAIEmbedder undefined`.

- [ ] **Step 3: Implement embedder**

Create `internal/memory/embedder.go`:

```go
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type OpenAIEmbedder struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

func NewOpenAIEmbedder(baseURL, model, apiKey string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		Client:  http.DefaultClient,
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}
type embedResponseItem struct {
	Embedding []float32 `json:"embedding"`
}
type embedResponse struct {
	Data []embedResponseItem `json:"data"`
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Model: e.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding server %d: %s", resp.StatusCode, string(msg))
	}
	var parsed embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([][]float32, len(parsed.Data))
	for i, item := range parsed.Data {
		out[i] = item.Embedding
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/memory/... -run Embedder`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/embedder.go internal/memory/embedder_test.go
git commit -m "feat(memory): OpenAIEmbedder posts /v1/embeddings (batched)

Single-request batch — the server controls per-request limits, not
the client. Bearer auth optional. Tests use httptest to assert the
body shape and the auth header without touching a real server."
```

---

## Task 8: VectorRetriever + PassthroughFusion

**Files:**
- Create: `internal/memory/retriever.go`
- Create: `internal/memory/fusion.go`
- Create: `internal/memory/retriever_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/retriever_test.go`:

```go
package memory

import (
	"context"
	"testing"
	"time"
)

func TestVectorRetriever_FindsNearest(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	chunks := []Chunk{
		{ID: "near", Content: "alpha", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
		{ID: "far", Content: "beta", Embedding: []float32{0, 0, 0, 1}, PublishedAt: time.Now().UTC()},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	embedder := fakeEmbedder{[][]float32{{1, 0, 0, 0}}}
	r := NewVectorRetriever(conn, embedder)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) < 1 || got[0].ID != "near" {
		t.Fatalf("expected 'near' to be top result, got %+v", got)
	}
}

func TestPassthroughFusion_ReturnsFirstList(t *testing.T) {
	in := []RetrievedChunk{
		{Chunk: Chunk{ID: "a"}, Score: 1},
		{Chunk: Chunk{ID: "b"}, Score: 0.5},
	}
	out := PassthroughFusion{}.Fuse([][]RetrievedChunk{in}, 1)
	if len(out) != 1 || out[0].ID != "a" {
		t.Fatalf("passthrough must keep order and cap k: %+v", out)
	}
}

type fakeEmbedder struct{ vecs [][]float32 }

func (f fakeEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return f.vecs, nil
}
```

- [ ] **Step 2: Implement retriever + fusion**

Create `internal/memory/fusion.go`:

```go
package memory

type Fusion interface {
	Fuse(lists [][]RetrievedChunk, k int) []RetrievedChunk
}

type PassthroughFusion struct{}

func (PassthroughFusion) Fuse(lists [][]RetrievedChunk, k int) []RetrievedChunk {
	if len(lists) == 0 {
		return nil
	}
	first := lists[0]
	if k <= 0 || k >= len(first) {
		return first
	}
	return first[:k]
}
```

Create `internal/memory/retriever.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Retriever interface {
	Retrieve(ctx context.Context, q Query, depth int) ([]RetrievedChunk, error)
}

type VectorRetriever struct {
	conn     *pgx.Conn
	embedder Embedder
}

func NewVectorRetriever(conn *pgx.Conn, e Embedder) *VectorRetriever {
	return &VectorRetriever{conn: conn, embedder: e}
}

func (r *VectorRetriever) Retrieve(ctx context.Context, q Query, depth int) ([]RetrievedChunk, error) {
	if depth <= 0 {
		depth = 20
	}
	vecs, err := r.embedder.Embed(ctx, []string{q.Text})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding server returned no vector")
	}
	rows, err := r.conn.Query(ctx, `
		SELECT id, content, embedding, metadata, COALESCE(source,''),
		       published_at, 1.0 - (embedding <=> $1) AS score
		FROM chunks
		WHERE superseded_by IS NULL
		ORDER BY embedding <=> $1
		LIMIT $2
	`, pgvector.NewVector(vecs[0]), depth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RetrievedChunk
	for rows.Next() {
		var rc RetrievedChunk
		var emb pgvector.Vector
		var meta []byte
		if err := rows.Scan(&rc.ID, &rc.Content, &emb, &meta, &rc.Source, &rc.PublishedAt, &rc.Score); err != nil {
			return nil, err
		}
		rc.Embedding = emb.Slice()
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &rc.Metadata)
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Run tests**

Run: `MEMORY_DATABASE_URL=postgres://... go test ./internal/memory/... -run "Retriever|Fusion"`
Expected: PASS (retriever) + PASS (passthrough). Retriever test skips without Postgres.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/retriever.go internal/memory/fusion.go internal/memory/retriever_test.go
git commit -m "feat(memory): VectorRetriever + PassthroughFusion (Phase 0 only)

Vector-only query per the spec's Phase 0 form — no BM25 CTE, no
freshness clauses, no recency term. The query filters superseded_by
IS NULL preemptively so Phase 2 supersession is a no-op SQL change.
PassthroughFusion satisfies the Fusion interface without merging."
```

---

## Task 9: BudgetContextBuilder

**Files:**
- Create: `internal/memory/context_builder.go`
- Create: `internal/memory/context_builder_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/context_builder_test.go`:

```go
package memory

import (
	"strings"
	"testing"
)

func TestBudgetContextBuilder_RespectsTokenBudget(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 60}
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: strings.Repeat("x", 30)}},
		{Chunk: Chunk{ID: "b", Content: strings.Repeat("y", 30)}},
		{Chunk: Chunk{ID: "c", Content: strings.Repeat("z", 30)}},
	}
	pc := cb.Build(Query{Text: "q"}, chunks)
	if len(pc.SourceIDs) != 2 {
		t.Fatalf("expected 2 chunks within budget, got %v", pc.SourceIDs)
	}
	if !strings.Contains(pc.User, "q") {
		t.Fatalf("question must appear in user prompt: %q", pc.User)
	}
}

func TestBudgetContextBuilder_AlwaysIncludesAtLeastOne(t *testing.T) {
	cb := BudgetContextBuilder{MaxRunes: 10}
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: strings.Repeat("x", 100)}},
	}
	pc := cb.Build(Query{Text: "q"}, chunks)
	if len(pc.SourceIDs) != 1 {
		t.Fatalf("must include at least one chunk even over budget")
	}
}
```

- [ ] **Step 2: Implement context builder**

Create `internal/memory/context_builder.go`:

```go
package memory

import (
	"fmt"
	"strings"
)

type ContextBuilder interface {
	Build(q Query, chunks []RetrievedChunk) PromptContext
}

// BudgetContextBuilder packs chunks into a rune-budgeted prompt and tracks the
// source ids so the Generator can produce citations the caller can verify.
type BudgetContextBuilder struct {
	MaxRunes int
}

func (b BudgetContextBuilder) Build(q Query, chunks []RetrievedChunk) PromptContext {
	var body strings.Builder
	var ids []string
	used := 0
	for i, c := range chunks {
		piece := fmt.Sprintf("\n[source:%s]\n%s\n", c.ID, c.Content)
		if i > 0 && used+len([]rune(piece)) > b.MaxRunes && b.MaxRunes > 0 {
			break
		}
		body.WriteString(piece)
		ids = append(ids, c.ID)
		used += len([]rune(piece))
	}
	return PromptContext{
		System:    "You are a careful assistant. Answer using only the provided sources and cite them by id in square brackets like [source:id].",
		User:      fmt.Sprintf("Sources:\n%s\n\nQuestion: %s", body.String(), q.Text),
		SourceIDs: ids,
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/memory/... -run ContextBuilder`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/context_builder.go internal/memory/context_builder_test.go
git commit -m "feat(memory): BudgetContextBuilder packs chunks with citation hints

Rune-budgeted prompt assembly. Each source carries a [source:id]
header so the Generator can be instructed to cite by that id and
the caller can verify citations against PromptContext.SourceIDs."
```

---

## Task 10: OpenAIGenerator

**Files:**
- Create: `internal/memory/generator.go`
- Create: `internal/memory/generator_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/generator_test.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIGenerator_ParsesCitationsAndAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("path: %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "llm-x" {
			t.Errorf("model: %v", body["model"])
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "The answer is X [source:c1] [source:c2]."}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := NewOpenAIGenerator(srv.URL, "llm-x", "tok")
	ans, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{
		System: "sys", User: "usr", SourceIDs: []string{"c1", "c2", "c3"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(ans.Text, "X") {
		t.Fatalf("answer text: %q", ans.Text)
	}
	if len(ans.Citations) != 2 || ans.Citations[0] != "c1" || ans.Citations[1] != "c2" {
		t.Fatalf("citations: %v", ans.Citations)
	}
}

func TestOpenAIGenerator_NoCitationsReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"plain answer"}}]}`))
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	ans, _ := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{SourceIDs: []string{"c1"}})
	if len(ans.Citations) != 0 {
		t.Fatalf("expected no citations, got %v", ans.Citations)
	}
}

func TestOpenAIGenerator_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	g := NewOpenAIGenerator(srv.URL, "m", "")
	if _, err := g.Generate(context.Background(), Query{Text: "q"}, PromptContext{}); err == nil {
		t.Fatalf("expected error on 502")
	}
}
```

- [ ] **Step 2: Implement generator**

Create `internal/memory/generator.go`:

```go
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type Generator interface {
	Generate(ctx context.Context, q Query, ctxBody PromptContext) (Answer, error)
}

type OpenAIGenerator struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

func NewOpenAIGenerator(baseURL, model, apiKey string) *OpenAIGenerator {
	return &OpenAIGenerator{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		Client:  http.DefaultClient,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

var citationRe = regexp.MustCompile(`\[source:([^\]]+)\]`)

func (g *OpenAIGenerator) Generate(ctx context.Context, q Query, pc PromptContext) (Answer, error) {
	reqBody := chatRequest{
		Model: g.Model,
		Messages: []chatMessage{
			{Role: "system", Content: pc.System},
			{Role: "user", Content: pc.User},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return Answer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return Answer{}, fmt.Errorf("llm server %d: %s", resp.StatusCode, string(msg))
	}
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Answer{}, err
	}
	if len(parsed.Choices) == 0 {
		return Answer{}, fmt.Errorf("llm returned no choices")
	}
	text := parsed.Choices[0].Message.Content
	matches := citationRe.FindAllStringSubmatch(text, -1)
	allowed := map[string]bool{}
	for _, id := range pc.SourceIDs {
		allowed[id] = true
	}
	var cites []string
	seen := map[string]bool{}
	for _, m := range matches {
		id := m[1]
		if !allowed[id] || seen[id] {
			continue
		}
		seen[id] = true
		cites = append(cites, id)
	}
	return Answer{Text: text, Citations: cites}, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/memory/... -run Generator`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/generator.go internal/memory/generator_test.go
git commit -m "feat(memory): OpenAIGenerator with citation extraction

Parses [source:id] markers from the LLM response and intersects
with PromptContext.SourceIDs, so the caller only sees citations
that actually correspond to retrieved chunks (defends against the
model hallucinating source ids)."
```

---

## Task 11: Orchestrator + Module factory

**Files:**
- Create: `internal/memory/orchestrator.go`
- Create: `internal/memory/module.go`
- Create: `internal/memory/orchestrator_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/orchestrator_test.go`:

```go
package memory

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	upserted []Chunk
}

func (f *fakeStore) Upsert(_ context.Context, chunks []Chunk) error {
	f.upserted = append(f.upserted, chunks...)
	return nil
}
func (f *fakeStore) Get(_ context.Context, _ []string) ([]Chunk, error) { return nil, nil }
func (f *fakeStore) MarkSuperseded(_ context.Context, _, _ string) error  { return nil }

type fakeRetriever struct{}

func (fakeRetriever) Retrieve(_ context.Context, q Query, _ int) ([]RetrievedChunk, error) {
	return []RetrievedChunk{
		{Chunk: Chunk{ID: "c1", Content: "alpha"}, Score: 0.9},
	}, nil
}

type fakeGenerator struct{}

func (fakeGenerator) Generate(_ context.Context, _ Query, pc PromptContext) (Answer, error) {
	return Answer{Text: "the answer [source:c1]", Citations: []string{"c1"}}, nil
}

func TestOrchestrator_IngestEmbedsAndStores(t *testing.T) {
	store := &fakeStore{}
	o := &Orchestrator{
		Chunker:        FixedSizeChunker{MaxRunes: 10, Overlap: 0},
		Embedder:       fakeEmbedder{vecs: [][]float32{{1, 0}, {0, 1}}},
		Store:          store,
		Retriever:      fakeRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d1", Text: "abcdefghij1234567890", Source: "s",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(store.upserted) != 2 {
		t.Fatalf("expected 2 chunks stored, got %d", len(store.upserted))
	}
	if len(store.upserted[0].Embedding) != 2 {
		t.Fatalf("embedding not attached: %+v", store.upserted[0])
	}
}

func TestOrchestrator_AnswerReturnsCitedAnswer(t *testing.T) {
	o := &Orchestrator{
		Retriever:      fakeRetriever{},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 1000},
		Generator:      fakeGenerator{},
		Depth:          5,
		FinalK:         3,
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q", K: 1})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(ans.Citations) != 1 || ans.Citations[0] != "c1" {
		t.Fatalf("expected citation c1, got %v", ans.Citations)
	}
}

func TestOrchestrator_IngestSetsPublishedAtFromSource(t *testing.T) {
	store := &fakeStore{}
	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: fakeEmbedder{vecs: [][]float32{{1}}},
		Store:    store,
	}
	err := o.Ingest(context.Background(), RawDocument{
		ID: "d", Text: "tiny", Source: "s",
		Metadata: map[string]any{"published_at": when.Format(time.RFC3339)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.upserted[0].PublishedAt.Equal(when) {
		t.Fatalf("published_at not honored: got %v want %v", store.upserted[0].PublishedAt, when)
	}
}
```

- [ ] **Step 2: Implement orchestrator**

Create `internal/memory/orchestrator.go`:

```go
package memory

import (
	"context"
	"fmt"
	"time"
)

// Orchestrator drives the ingestion and query paths. Every dependency is an
// interface; the wiring lives in Module so swapping (e.g. switching to a
// HybridRetriever in Phase 1) is a Module-level config change, not an
// Orchestrator code edit.
type Orchestrator struct {
	Chunker        Chunker
	Embedder       Embedder
	Store          Store
	Retriever      Retriever
	Fusion         Fusion
	ContextBuilder ContextBuilder
	Generator      Generator
	Depth          int
	FinalK         int
}

func (o *Orchestrator) Ingest(ctx context.Context, doc RawDocument) error {
	published := time.Now().UTC()
	if raw, ok := doc.Metadata["published_at"].(string); ok && raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			published = t
		}
	}
	parsed := ParsedDocument{
		ID:          doc.ID,
		Source:      doc.Source,
		Text:        doc.Text,
		Metadata:    doc.Metadata,
		PublishedAt: published,
	}
	chunks := o.Chunker.Chunk(parsed)
	if len(chunks) == 0 {
		return nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	vecs, err := o.Embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vecs) != len(chunks) {
		return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vecs), len(chunks))
	}
	for i := range chunks {
		chunks[i].Embedding = vecs[i]
	}
	return o.Store.Upsert(ctx, chunks)
}

func (o *Orchestrator) Answer(ctx context.Context, q Query) (Answer, error) {
	depth := o.Depth
	if depth <= 0 {
		depth = 20
	}
	candidates, err := o.Retriever.Retrieve(ctx, q, depth)
	if err != nil {
		return Answer{}, fmt.Errorf("retrieve: %w", err)
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = o.FinalK
	}
	fused := o.Fusion.Fuse([][]RetrievedChunk{candidates}, finalK)
	prompt := o.ContextBuilder.Build(q, fused)
	return o.Generator.Generate(ctx, q, prompt)
}
```

Create `internal/memory/module.go`:

```go
package memory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// NewModule wires a production Orchestrator from MemoryConfig. It opens a pgx
// connection, applies migrations, and constructs every concrete implementation.
// Callers should hold on to the returned *pgx.Conn (so they can close it on
// shutdown) and the Orchestrator (the only thing they call into).
func NewModule(ctx context.Context, cfg MemoryConfig) (*Orchestrator, *pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := ApplyMigrations(ctx, conn, cfg.EmbeddingDim); err != nil {
		_ = conn.Close(ctx)
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	embedder := NewOpenAIEmbedder(cfg.EmbeddingBaseURL, cfg.EmbeddingModel, cfg.EmbeddingAPIKey)
	generator := NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, "")
	// Token→rune conversion: most English tokenizers land near 4 chars/token, and
	// we use rune-counts internally (see Chunker). The default 700-token /
	// 100-overlap config maps to ~2800/400 runes — slightly conservative, which
	// helps stay under embedding API per-request limits.
	const runesPerToken = 4
	return &Orchestrator{
		Chunker: FixedSizeChunker{
			MaxRunes: cfg.ChunkSizeTokens * runesPerToken,
			Overlap:  cfg.ChunkOverlapTokens * runesPerToken,
		},
		Embedder:       embedder,
		Store:          NewPgStore(conn),
		Retriever:      NewVectorRetriever(conn, embedder),
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Generator:      generator,
		Depth:          20,
		FinalK:         cfg.TopK,
	}, conn, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/memory/... -run Orchestrator`
Expected: PASS (all three orchestrator tests).

- [ ] **Step 4: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/module.go internal/memory/orchestrator_test.go
git commit -m "feat(memory): Orchestrator + Module factory

Orchestrator.Ingest pipes RawDocument → chunks → embeddings →
Store.Upsert; honors metadata.published_at when present (Phase 2
will rely on it). Orchestrator.Answer routes Retriever → Fusion →
ContextBuilder → Generator. Module wires production components
from MemoryConfig and applies migrations on startup."
```

---

## Task 12: cmd/memory-demo end-to-end CLI

**Files:**
- Create: `cmd/memory-demo/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Implement the demo CLI**

Create `cmd/memory-demo/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/luannn010/ptolemy/internal/memory"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cfg, err := memory.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "module:", err)
		os.Exit(2)
	}
	defer conn.Close(ctx)

	switch os.Args[1] {
	case "ingest":
		if len(os.Args) != 4 {
			usage()
			os.Exit(1)
		}
		id := os.Args[2]
		path := os.Args[3]
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			os.Exit(2)
		}
		if err := orch.Ingest(ctx, memory.RawDocument{ID: id, Source: path, Text: string(data)}); err != nil {
			fmt.Fprintln(os.Stderr, "ingest:", err)
			os.Exit(2)
		}
		fmt.Println("ingested:", id)
	case "ask":
		if len(os.Args) != 3 {
			usage()
			os.Exit(1)
		}
		ans, err := orch.Answer(ctx, memory.Query{Text: os.Args[2], K: 5})
		if err != nil {
			fmt.Fprintln(os.Stderr, "answer:", err)
			os.Exit(2)
		}
		fmt.Println(ans.Text)
		if len(ans.Citations) > 0 {
			fmt.Println("\ncitations:", ans.Citations)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  memory-demo ingest <doc-id> <path-to-file>")
	fmt.Fprintln(os.Stderr, "  memory-demo ask    \"<question>\"")
}
```

- [ ] **Step 2: Modify Makefile**

Edit `Makefile` build target to add the new binary. Locate the lines:

```
build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/workerd ./cmd/workerd
	go build -o $(BIN_DIR)/ptolemy-mcp ./cmd/ptolemy-mcp
	go build -o $(BIN_DIR)/policy-demo ./cmd/policy-demo
```

Append one line:

```
	go build -o $(BIN_DIR)/memory-demo ./cmd/memory-demo
```

- [ ] **Step 3: Verify the binary compiles**

Run: `make build`
Expected: all four binaries produced, including `bin/memory-demo`.

- [ ] **Step 4: Smoke-test against a real stack (manual; document only)**

This step is run by the engineer locally, not by CI. Document the exact commands in the commit message so future engineers can reproduce.

```bash
# Prereqs in .env (matches the names already in .env.example):
#   DATABASE_URL=postgres://ptolemy:ptolemy@localhost:5432/ptolemy?sslmode=disable
#   BRAIN_BASE_URL=http://127.0.0.1:8088
#   BRAIN_MODEL=gemma-4-e2b
#   RAG_TOP_K=8
#   RAG_CHUNK_SIZE_TOKENS=700
#   RAG_CHUNK_OVERLAP_TOKENS=100
# Plus the new embedding vars (add to .env.example in Task 14):
#   EMBEDDING_BASE_URL=http://localhost:8000
#   EMBEDDING_MODEL=bge-large-en-v1.5
#   EMBEDDING_DIM=1024
#   EMBEDDING_API_KEY=
echo "Ptolemy is a Go-based RAG memory and agent runtime project." > /tmp/doc1.txt
./bin/memory-demo ingest doc1 /tmp/doc1.txt
./bin/memory-demo ask "What is Ptolemy?"
# Expected: an answer mentioning Go/RAG with [source:doc1#0] in the
# citations line.
```

- [ ] **Step 5: Commit**

```bash
git add cmd/memory-demo/main.go Makefile
git commit -m "feat(memory): cmd/memory-demo CLI for end-to-end smoke tests

ingest <id> <path>  reads a file, embeds it, and writes it to Postgres.
ask    <question>   runs Retriever → Fusion → ContextBuilder →
                    Generator and prints answer + citations.

Used to satisfy the spec's Phase 0 acceptance criterion: a small
corpus + a question returns a grounded answer with at least one
correct citation."
```

---

## Task 13: Wire memory env vars into `internal/config`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

This task exposes the memory env vars through the shared `internal/config.Config` so future workers (workerd, ptolemy-mcp) can read them without re-parsing env. Phase 0 itself uses `memory.LoadConfig()`; this is the bridge for Phase 1+.

- [ ] **Step 1: Add fields to Config**

In `internal/config/config.go`, inside the `Config` struct definition, add (DATABASE_URL replaces SQLite-only DB_PATH for memory-aware callers; existing SQLite `DBPath` stays untouched for sessions/command logs):

```go
DatabaseURL       string
EmbeddingBaseURL  string
EmbeddingModel    string
EmbeddingDim      int
EmbeddingAPIKey   string
RagTopK           int
RagChunkSize      int
RagChunkOverlap   int
```

In `Load()`, after the existing assignments, add:

```go
cfg.DatabaseURL = getEnv("DATABASE_URL", "")
cfg.EmbeddingBaseURL = getEnv("EMBEDDING_BASE_URL", "")
cfg.EmbeddingModel = getEnv("EMBEDDING_MODEL", "")
cfg.EmbeddingDim = getEnvInt("EMBEDDING_DIM", 0)
cfg.EmbeddingAPIKey = getEnv("EMBEDDING_API_KEY", "")
cfg.RagTopK = getEnvInt("RAG_TOP_K", 8)
cfg.RagChunkSize = getEnvInt("RAG_CHUNK_SIZE_TOKENS", 700)
cfg.RagChunkOverlap = getEnvInt("RAG_CHUNK_OVERLAP_TOKENS", 100)
```

No defaults that hide endpoint misconfiguration; missing memory vars become an explicit error only inside `memory.LoadConfig()`. RAG knobs get sensible defaults matching .env.example.

- [ ] **Step 2: Extend config tests**

In `internal/config/config_test.go`, add a test that round-trips the memory vars:

```go
func TestLoadConfigCarriesMemoryVars(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("RAG_TOP_K", "12")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://x" || cfg.EmbeddingDim != 768 || cfg.RagTopK != 12 {
		t.Fatalf("memory vars not loaded: %+v", cfg)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): surface memory env vars on shared Config

DATABASE_URL, EMBEDDING_*, RAG_* are read by config.Load but kept
default-free for endpoints. memory.LoadConfig owns strict validation;
this surface only exists so workerd and future delivery code can
read the values without re-parsing env. The existing DBPath
(SQLite) stays untouched — sessions/command logs are not moving."
```

---

## Task 14: Update `.env.example` with embedding vars

**Files:**
- Modify: `.env.example`

- [ ] **Step 1: Append the four new vars**

The existing `.env.example` already has `DATABASE_URL`, `BRAIN_BASE_URL`, `BRAIN_MODEL`, `RAG_TOP_K`, `RAG_CHUNK_SIZE_TOKENS`, `RAG_CHUNK_OVERLAP_TOKENS`. Append:

```
EMBEDDING_BASE_URL=http://localhost:8000
EMBEDDING_MODEL=bge-large-en-v1.5
EMBEDDING_DIM=1024
EMBEDDING_API_KEY=
```

The values are placeholders — the engineer overrides them in their real `.env` to match the embedding server they've deployed. The model and dim must agree (e.g. `bge-large-en-v1.5` → 1024, `text-embedding-3-small` → 1536).

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add .env.example
git commit -m "chore(env): document memory embedding env vars in .env.example

EMBEDDING_BASE_URL/MODEL/DIM/API_KEY are required by the memory
module. Values are placeholders; override in real .env per the
deployed embedding server."
```

---

## Final: Open PR to main

- [ ] **Step 1: Verify**

```bash
make build && make test
go test ./internal/memory/...
```

If `MEMORY_DATABASE_URL` is unset locally, the Postgres-dependent tests skip — that is expected. CI will fail until either Postgres is provisioned in the workflow or the integration tests are gated by build tag (see follow-up below).

- [ ] **Step 2: Push the branch**

```bash
git push -u origin ptolemy/memory-phase0
```

- [ ] **Step 3: Open the PR**

Per AGENTS.md, the user opens the PR (the GitHub plugin tooling isn't available in this session). PR title:

> `feat(memory): Phase 0 — vector RAG MVP (Postgres + pgvector)`

PR body skeleton (fill the `.github/pull_request_template.md`):

```markdown
## Summary
- Adds internal/memory: Postgres+pgvector store, OpenAI-compatible embedder + generator, fixed-size chunker, vector retriever, passthrough fusion, budget-aware context builder, orchestrator.
- Adds cmd/memory-demo CLI for end-to-end smoke testing.
- Adds 8 memory env vars (MEMORY_DATABASE_URL, EMBEDDING_*, LLM_*).
- Postgres-dependent tests skip when MEMORY_DATABASE_URL is unset.

## Spec & plan
- Spec: docs/memory/ (5 files)
- Plan: docs/superpowers/plans/2026-05-27-memory-phase0.md

## Acceptance (spec §IMPLEMENTATION_PLAN Phase 0)
- [x] Pipeline ingest→chunk→embed→store→retrieve→build→generate compiles and unit-tests green.
- [x] Swapping retriever is a Module-level config change, not a code edit.
- [x] Integration smoke documented in commit Task 12 step 4.
```

- [ ] **Step 4: Follow-ups (NOT in this PR)**

Capture in the PR description, do not implement here:
- Provision Postgres+pgvector in `.github/workflows/go-ci.yml` so integration tests run on CI (or move them behind `//go:build integration` and add to the existing integration-tests job).
- Phase 1: BM25 backend selection, HybridRetriever, RrfFusion, eval harness.

---

## Self-Review

**Env var naming alignment (after the .env.example pass):**
- Reuse: `DATABASE_URL`, `BRAIN_BASE_URL`, `BRAIN_MODEL`, `RAG_TOP_K`, `RAG_CHUNK_SIZE_TOKENS`, `RAG_CHUNK_OVERLAP_TOKENS`.
- New: `EMBEDDING_BASE_URL`, `EMBEDDING_MODEL`, `EMBEDDING_DIM`, `EMBEDDING_API_KEY`.

**Spec coverage (against `docs/memory/IMPLEMENTATION_PLAN.md` Phase 0 checklist):**
- ✅ `0001_chunks_core` migration → Task 4
- ✅ Loader (single source) → covered implicitly: `cmd/memory-demo` reads files; no separate Loader struct needed yet (spec says "for the single most common source" — file IO is that source)
- ✅ Parser → Phase 0 has no PDF/HTML; raw text passes through `Orchestrator.Ingest` unchanged. Marked as a follow-up rather than introducing a no-op type.
- ✅ Chunker → Task 6
- ✅ Embedder → Task 7
- ✅ Store.upsert / get → Task 5
- ✅ VectorRetriever → Task 8
- ✅ PassthroughFusion → Task 8
- ✅ ContextBuilder w/ token budget → Task 9
- ✅ Generator w/ citations → Task 10
- ✅ Orchestrator wiring from config → Tasks 11 + 13

**Spec design rules:**
- ✅ Vector index covers the same chunk rows — there's only one table.
- ✅ All SQL is parameterized (`$1`, `$2`, …).
- ✅ Top-k is not the lever for quality — `Depth` (20) is separate from `FinalK` (5) as the spec asks.
- ✅ Embedding is batched; the query path doesn't ingest.
- ✅ Phase 0 acceptance tests are covered by Task 12 step 4.

**Placeholder scan:** no "TBD", no "fill in later", no "similar to Task N" shortcuts. Every step contains the full code an engineer would type.

**Type consistency check:**
- `Chunk.Embedding []float32` used consistently in Store, Retriever, Embedder, Orchestrator.
- `Embedder.Embed` returns `[][]float32` everywhere.
- `Orchestrator.Depth` and `Orchestrator.FinalK` are the two knobs; no rename across tasks.
- `PromptContext.SourceIDs` (with `s`) used in ContextBuilder, Generator, and Orchestrator tests.

No gaps found.

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-27-memory-phase0.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch with checkpoints.

**Which approach?**
