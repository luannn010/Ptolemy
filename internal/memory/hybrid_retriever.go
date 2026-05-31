package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog/log"
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
	decayLambda     float64
	aliasExpansion  bool
}

func NewHybridRetriever(conn *pgx.Conn, e Embedder, recencyWeight float64, recencyHalfLife time.Duration) *HybridRetriever {
	return &HybridRetriever{
		conn:            conn,
		embedder:        e,
		recencyWeight:   recencyWeight,
		recencyHalfLife: recencyHalfLife,
		decayLambda:     0.05,
	}
}

// WithDecayLambda overrides the project-row decay dial (SPEC-GC §4 default 0.05).
func (r *HybridRetriever) WithDecayLambda(l float64) *HybridRetriever { r.decayLambda = l; return r }

// WithAliasExpansion toggles lexical-arm synonym expansion (default off).
func (r *HybridRetriever) WithAliasExpansion(on bool) *HybridRetriever {
	r.aliasExpansion = on
	return r
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
       c.subject_id, c.session_id, c.project_id,
       b.rank AS bm25_rank, v.rank AS vec_rank,
       ( COALESCE(1.0 / (60 + b.rank), 0)
       + COALESCE(1.0 / (60 + v.rank), 0)
       + $6 * exp(-extract(epoch FROM $5 - c.published_at) / $7) )
     * CASE WHEN c.scope = 'project' AND NOT c.pinned
            THEN c.importance * exp(-$10::float8 * extract(epoch FROM now() - c.last_accessed_at) / 86400
                                    / (1 + c.access_count))
            ELSE 1.0 END AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.status = 'active'
  AND c.published_at <= $5
  AND (c.subject_id IS NULL OR c.subject_id = $8)
  -- project filter lives ONLY here (outer), not in the @@@ CTE: ParadeDB rejects a
  -- pure-parameter predicate ($9::text IS NULL) inside a @@@ query as an unsupported
  -- shape. The outer filter still enforces correctness (no cross-project leak) and is
  -- a no-op when $9 is NULL, keeping the all-global eval byte-identical.
  AND ($9::text IS NULL OR c.project_id IS NULL OR c.project_id = $9)
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
	embedStart := time.Now()
	vecs, err := r.embedder.Embed(ctx, []string{q.Text})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding server returned no vector")
	}
	log.Info().Str("stage", "embed").Int64("ms", time.Since(embedStart).Milliseconds()).Int("dim", len(vecs[0])).Msg("retriever: query embedded")
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
	var proj any
	if q.ProjectID != nil {
		proj = *q.ProjectID
	}
	bm25Text := q.Text
	if r.aliasExpansion {
		bm25Text = expandAliases(q.Text)
	}
	queryStart := time.Now()
	rows, err := r.conn.Query(ctx, hybridRrfQuery,
		bm25Text,
		pgvector.NewVector(vecs[0]),
		depth,
		finalK,
		asOf,
		r.recencyWeight,
		r.recencyHalfLife.Seconds(),
		subj,
		proj,
		r.decayLambda,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	debug := log.Debug().Enabled()
	var out []RetrievedChunk
	for rows.Next() {
		var rc RetrievedChunk
		var meta []byte
		var bm25Rank, vecRank *int // NULL when that arm did not match this row
		if err := rows.Scan(&rc.ID, &rc.Content, &meta, &rc.Source, &rc.PublishedAt,
			&rc.SubjectID, &rc.SessionID, &rc.ProjectID, &bm25Rank, &vecRank, &rc.Score); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &rc.Metadata)
		}
		if debug {
			// "arm" shows which retrieval key matched: both / bm25 (lexical) /
			// vector (semantic). Ranks are 1-based within each arm.
			arm := "vector"
			switch {
			case bm25Rank != nil && vecRank != nil:
				arm = "both"
			case bm25Rank != nil:
				arm = "bm25"
			}
			kind, _ := rc.Metadata["kind"].(string)
			log.Debug().
				Str("stage", "retrieved").
				Int("n", len(out)).
				Str("id", rc.ID).
				Float64("score", rc.Score).
				Str("arm", arm).
				Interface("bm25_rank", bm25Rank).
				Interface("vec_rank", vecRank).
				Str("kind", kind).
				Str("content", preview(rc.Content, 80)).
				Msg("retriever: candidate")
		}
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ev := log.Info()
	if len(out) == 0 {
		ev = log.Warn()
	}
	ev.Str("stage", "vector_search").
		Int64("query_ms", time.Since(queryStart).Milliseconds()).
		Int("rows", len(out)).
		Int("depth", depth).
		Str("subject", fmt.Sprintf("%v", subj)).
		Str("project", fmt.Sprintf("%v", proj)).
		Msg("retriever: hybrid SQL (BM25+vector RRF) done")
	return out, nil
}

// preview truncates content to n runes for single-line log output, collapsing
// newlines so a chunk stays on one log line.
func preview(s string, n int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
