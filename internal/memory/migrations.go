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
