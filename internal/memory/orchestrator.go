package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
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
	scope := "global"
	if raw, ok := doc.Metadata["scope"].(string); ok && raw != "" {
		scope = raw
	}
	confidence := "normal"
	if raw, ok := doc.Metadata["confidence"].(string); ok && raw != "" {
		confidence = raw
	}
	factSubject, _ := doc.Metadata["fact_subject"].(string)
	factPredicate, _ := doc.Metadata["fact_predicate"].(string)
	for i := range chunks {
		chunks[i].Scope = scope
		chunks[i].Confidence = confidence
		if factSubject != "" && factPredicate != "" {
			fs, fp := factSubject, factPredicate
			chunks[i].FactSubject = &fs
			chunks[i].FactPredicate = &fp
		}
	}

	if old, ok := doc.Metadata["supersedes"].(string); ok && old != "" {
		return o.Store.SupersedeOnUpsert(ctx, chunks, old)
	}

	// Structured-fact ladder (SPEC §5 step 1, ~0ms, one indexed lookup).
	if factSubject != "" && factPredicate != "" {
		existing, found, err := o.Store.LookupFact(ctx, factSubject, factPredicate)
		if err != nil {
			return fmt.Errorf("lookup fact: %w", err)
		}
		if found {
			if normalizeContent(existing.Content) == normalizeContent(chunks[0].Content) {
				return o.Store.Reinforce(ctx, []string{existing.ID}) // duplicate
			}
			return o.Store.Supersede(ctx, chunks, existing.ID) // correction
		}
	}
	return o.Store.Upsert(ctx, chunks)
}

func (o *Orchestrator) Answer(ctx context.Context, q Query) (Answer, error) {
	depth := o.Depth
	if depth <= 0 {
		depth = 20
	}
	// Resolve AsOf default once; downstream retrievers can rely on it being
	// non-nil so they don't each have to default it on their own.
	asOf := time.Now().UTC()
	if q.AsOf != nil {
		asOf = *q.AsOf
	}
	local := q
	local.AsOf = &asOf

	candidates, err := o.Retriever.Retrieve(ctx, local, depth)
	if err != nil {
		return Answer{}, fmt.Errorf("retrieve: %w", err)
	}
	if o.Store != nil && len(candidates) > 0 {
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		if err := o.Store.Reinforce(ctx, ids); err != nil {
			log.Warn().Err(err).Msg("reinforce failed; serving answer anyway")
		}
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = o.FinalK
	}
	fused := o.Fusion.Fuse([][]RetrievedChunk{candidates}, finalK)
	prompt := o.ContextBuilder.Build(q, fused)
	return o.Generator.Generate(ctx, q, prompt)
}
