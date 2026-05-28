package memory

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Bm25Retriever performs lexical retrieval via ParadeDB pg_search.
// The @@@ operator runs a Tantivy BM25 query against the chunks_content_bm25
// index; paradedb.score(id) returns the per-row BM25 score and is also used
// to order the result set.
type Bm25Retriever struct {
	conn *pgx.Conn
}

func NewBm25Retriever(conn *pgx.Conn) *Bm25Retriever {
	return &Bm25Retriever{conn: conn}
}

func (r *Bm25Retriever) Retrieve(ctx context.Context, q Query, depth int) ([]RetrievedChunk, error) {
	if depth <= 0 {
		depth = 20
	}
	if strings.TrimSpace(q.Text) == "" {
		return nil, nil
	}
	// Orchestrator.Answer always passes a non-nil AsOf, but standalone callers
	// (ARCHITECTURE.md "Option B" — Bm25Retriever used directly without the
	// hybrid orchestrator) may pass Query.AsOf == nil. The local fallback keeps
	// the published_at <= $2 parameter always bound; do not remove as duplicate
	// of Orchestrator.Answer's defaulting.
	asOf := time.Now().UTC()
	if q.AsOf != nil {
		asOf = *q.AsOf
	}
	rows, err := r.conn.Query(ctx, `
		SELECT id, content, metadata, COALESCE(source,''), published_at,
		       paradedb.score(id) AS score
		FROM chunks
		WHERE content @@@ $1
		  AND superseded_by IS NULL
		  AND status = 'active'
		  AND published_at <= $2
		ORDER BY paradedb.score(id) DESC
		LIMIT $3
	`, q.Text, asOf, depth)
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
