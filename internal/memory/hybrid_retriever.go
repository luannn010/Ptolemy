package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// HybridRetriever runs the spec's Option A: a single SQL query with both
// retrieval arms as CTEs (vector via pgvector <=>, lexical via pg_search @@@),
// fused inside Postgres with RRF (C=60). Phase 2 freshness filters and the
// recency term are intentionally absent — they land in Phase 2.
type HybridRetriever struct {
	conn     *pgx.Conn
	embedder Embedder
}

func NewHybridRetriever(conn *pgx.Conn, e Embedder) *HybridRetriever {
	return &HybridRetriever{conn: conn, embedder: e}
}

const hybridRrfQuery = `
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE superseded_by IS NULL
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0) + COALESCE(1.0 / (60 + v.rank), 0) AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
ORDER BY score DESC
LIMIT $4
`

func (r *HybridRetriever) Retrieve(ctx context.Context, q Query, depth int) ([]RetrievedChunk, error) {
	if depth <= 0 {
		depth = 20
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = depth
	}
	vecs, err := r.embedder.Embed(ctx, []string{q.Text})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding server returned no vector")
	}
	rows, err := r.conn.Query(ctx, hybridRrfQuery, q.Text, pgvector.NewVector(vecs[0]), depth, finalK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RetrievedChunk
	for rows.Next() {
		var rc RetrievedChunk
		var meta []byte
		if err := rows.Scan(&rc.ID, &rc.Content, &meta, &rc.Source, &rc.PublishedAt, &rc.Score); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &rc.Metadata)
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
