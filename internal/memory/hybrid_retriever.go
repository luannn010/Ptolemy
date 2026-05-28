package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// HybridRetriever runs the spec's Option A: a single SQL query with both
// retrieval arms as CTEs (vector via pgvector <=>, lexical via pg_search @@@),
// fused inside Postgres with RRF (C=60). Phase 2 adds the freshness CTE
// filters (published_at <= $5) and the recency term in the outer SELECT;
// Phase 3 promotes the recency weight ($6) and halflife seconds ($7) to
// constructor parameters so callers can tune them from MemoryConfig.
type HybridRetriever struct {
	conn            *pgx.Conn
	embedder        Embedder
	recencyWeight   float64
	recencyHalfLife time.Duration
}

func NewHybridRetriever(conn *pgx.Conn, e Embedder, recencyWeight float64, recencyHalfLife time.Duration) *HybridRetriever {
	return &HybridRetriever{
		conn:            conn,
		embedder:        e,
		recencyWeight:   recencyWeight,
		recencyHalfLife: recencyHalfLife,
	}
}

const hybridRrfQuery = `
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND status = 'active'
      AND published_at <= $5
      AND (subject_id IS NULL OR subject_id = $8)
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE status = 'active'
      AND published_at <= $5
      AND (subject_id IS NULL OR subject_id = $8)
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0)
     + COALESCE(1.0 / (60 + v.rank), 0)
     + $6 * exp(-extract(epoch FROM $5 - c.published_at) / $7) AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.status = 'active'
  AND c.published_at <= $5
  AND (c.subject_id IS NULL OR c.subject_id = $8)
ORDER BY score DESC,
         -- project-only recency tiebreak; NULL for every non-project row, so it is
         -- a constant (no-op) on the all-global eval baseline and cannot reorder it.
         CASE WHEN c.scope = 'project' THEN c.last_accessed_at END DESC NULLS LAST
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
	// Orchestrator.Answer always passes a non-nil AsOf, but standalone callers
	// (ARCHITECTURE.md "Option B" — HybridRetriever used directly without the
	// hybrid orchestrator) may pass Query.AsOf == nil. The local fallback keeps
	// the published_at <= $5 parameter always bound; do not remove as duplicate
	// of Orchestrator.Answer's defaulting.
	asOf := time.Now().UTC()
	if q.AsOf != nil {
		asOf = *q.AsOf
	}
	var subj any
	if q.SubjectID != nil {
		subj = *q.SubjectID
	}
	rows, err := r.conn.Query(ctx, hybridRrfQuery,
		q.Text,
		pgvector.NewVector(vecs[0]),
		depth,
		finalK,
		asOf,
		r.recencyWeight,
		r.recencyHalfLife.Seconds(),
		subj,
	)
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
