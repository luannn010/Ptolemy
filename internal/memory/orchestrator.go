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
	if old, ok := doc.Metadata["supersedes"].(string); ok && old != "" {
		return o.Store.SupersedeOnUpsert(ctx, chunks, old)
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
