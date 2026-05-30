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

// RecallResult is the retrieval-only output of Recall: the assembled context
// (dual-circuit synthesis summary + supporting atoms, already budget-packed) and
// the source ids — WITHOUT an LLM generation pass. This is the fast path for
// "recall context" callers (hooks, MCP) who want the compressed context, not a
// freshly-worded answer.
type RecallResult struct {
	Context   string
	SourceIDs []string
}

// contextPacker is the optional capability a ContextBuilder may implement to
// expose its ordering + budget packing without the prompt framing. Same-package
// so MMRContextBuilder satisfies it via the unexported selectAndPack.
type contextPacker interface {
	selectAndPack(chunks []RetrievedChunk) (string, []string)
}

// Recall runs the retrieval path (retrieve → reinforce → fuse → select/pack)
// and returns the assembled context directly, skipping Generator.Generate. It
// shares ordering/budgeting with Answer's ContextBuilder so recalled context
// matches what the LLM would have seen — just without the generation latency.
func (o *Orchestrator) Recall(ctx context.Context, q Query) (RecallResult, error) {
	depth := o.Depth
	if depth <= 0 {
		depth = 20
	}
	asOf := time.Now().UTC()
	if q.AsOf != nil {
		asOf = *q.AsOf
	}
	local := q
	local.AsOf = &asOf

	recallStart := time.Now()
	retrieveStart := time.Now()
	candidates, err := o.Retriever.Retrieve(ctx, local, depth)
	if err != nil {
		return RecallResult{}, fmt.Errorf("retrieve: %w", err)
	}
	log.Info().Str("stage", "retrieve").Int64("ms", time.Since(retrieveStart).Milliseconds()).Int("candidates", len(candidates)).Msg("recall: retrieve done")

	if o.Store != nil && len(candidates) > 0 {
		reinforceStart := time.Now()
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		if err := o.Store.Reinforce(ctx, ids); err != nil {
			log.Warn().Err(err).Msg("reinforce failed; serving recall anyway")
		}
		log.Info().Str("stage", "reinforce").Int64("ms", time.Since(reinforceStart).Milliseconds()).Int("ids", len(ids)).Msg("recall: reinforce done")
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = o.FinalK
	}
	fused := o.Fusion.Fuse([][]RetrievedChunk{candidates}, finalK)

	packStart := time.Now()
	var packedIDs int
	defer func() {
		ev := log.Info()
		if packedIDs == 0 {
			ev = log.Warn()
		}
		ev.Str("stage", "pack").
			Int64("pack_ms", time.Since(packStart).Milliseconds()).
			Int64("total_ms", time.Since(recallStart).Milliseconds()).
			Int("packed", packedIDs).
			Msg("recall: select/pack done (no LLM)")
	}()

	if packer, ok := o.ContextBuilder.(contextPacker); ok {
		body, sourceIDs := packer.selectAndPack(fused)
		packedIDs = len(sourceIDs)
		return RecallResult{Context: body, SourceIDs: sourceIDs}, nil
	}
	// Fallback for builders without selectAndPack: derive context from the
	// PromptContext the builder produces.
	pc := o.ContextBuilder.Build(q, fused)
	return RecallResult{Context: pc.User, SourceIDs: pc.SourceIDs}, nil
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
