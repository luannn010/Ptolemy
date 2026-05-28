package memory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func requirePG(t *testing.T) string {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
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

	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE`)

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

	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("second ApplyMigrations: %v", err)
	}
}

func TestMigrationsFS_Contains0001(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := "0001_chunks_core.sql"
	found := false
	for _, n := range names {
		if n == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s in embedded migrations, got %v", want, names)
	}
}

func TestMigrationsFS_SubstitutesEmbeddingDim(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0001_chunks_core.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "__EMBEDDING_DIM__") {
		t.Fatalf("migration file must contain __EMBEDDING_DIM__ placeholder")
	}
	substituted := strings.ReplaceAll(string(data), "__EMBEDDING_DIM__", "1536")
	if strings.Contains(substituted, "__EMBEDDING_DIM__") {
		t.Fatalf("substitution did not replace all occurrences")
	}
	if !strings.Contains(substituted, "VECTOR(1536)") {
		t.Fatalf("expected VECTOR(1536) after substitution, got: %s", substituted)
	}
}

func TestApplyMigrations_CreatesBm25Index(t *testing.T) {
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE`)

	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var n int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='chunks' AND indexname='chunks_content_bm25'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected chunks_content_bm25 BM25 index to exist after migrations (got count=%d)", n)
	}
}

func TestMigrationsFS_Contains0002(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	want := "0002_chunks_bm25.sql"
	found := false
	for _, e := range entries {
		if e.Name() == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s in embedded migrations", want)
	}
}

func TestApplyMigrations_CreatesFreshnessIndexes(t *testing.T) {
	url := requirePG(t)
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	_, _ = conn.Exec(context.Background(), `DROP TABLE IF EXISTS chunks, chunk_audit, memory_schema_migrations CASCADE`)

	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var nPub, nLive int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='chunks' AND indexname='chunks_published_at'`,
	).Scan(&nPub); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='chunks' AND indexname='chunks_live'`,
	).Scan(&nLive); err != nil {
		t.Fatal(err)
	}
	if nPub != 1 {
		t.Fatalf("expected chunks_published_at index, got count=%d", nPub)
	}
	if nLive != 1 {
		t.Fatalf("expected chunks_live partial index, got count=%d", nLive)
	}
}

func TestMigrationsFS_Contains0003(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	want := "0003_chunks_freshness.sql"
	for _, e := range entries {
		if e.Name() == want {
			return
		}
	}
	t.Fatalf("expected %s in embedded migrations", want)
}

func TestMigrationsFS_Contains0004(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0004_chunks_gc_lifecycle.sql")
	if err != nil {
		t.Fatalf("0004 migration missing from embed FS: %v", err)
	}
	if !strings.Contains(string(data), "chunk_audit") {
		t.Fatalf("0004 should create chunk_audit")
	}
}

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
