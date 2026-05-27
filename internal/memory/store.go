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

	// SupersedeOnUpsert atomically upserts the new chunks AND marks every
	// chunk whose id starts with "<supersedesOldDocID>#" as superseded by the
	// representative new chunk (chunks[0].ID). Both writes are inside one
	// transaction; either both commit or neither does. Calling with a
	// supersedesOldDocID that matches no rows is not an error.
	SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error
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

func (s *PgStore) SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error {
	if len(chunks) == 0 {
		return fmt.Errorf("SupersedeOnUpsert: empty chunk slice")
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Defer Rollback; pgx makes a successful Commit a no-op for Rollback.
	defer func() { _ = tx.Rollback(ctx) }()

	for _, c := range chunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		_, err = tx.Exec(ctx, `
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

	// Representative new chunk; the retrieval filter only cares whether
	// superseded_by IS NULL, so a single pointer per supersession is enough.
	rep := chunks[0].ID

	if _, err := tx.Exec(ctx, `
		UPDATE chunks
		SET superseded_by = $1
		WHERE id LIKE $2 || '#%'
		  AND superseded_by IS NULL
	`, rep, supersedesOldDocID); err != nil {
		return fmt.Errorf("supersede %s: %w", supersedesOldDocID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
