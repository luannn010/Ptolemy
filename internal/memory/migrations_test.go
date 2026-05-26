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

	if err := ApplyMigrations(context.Background(), conn, 1024); err != nil {
		t.Fatalf("second ApplyMigrations: %v", err)
	}
}
