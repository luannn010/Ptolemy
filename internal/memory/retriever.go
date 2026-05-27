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
