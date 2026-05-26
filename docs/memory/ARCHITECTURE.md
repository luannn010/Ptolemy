# Architecture

The module is a set of small components, each behind a narrow interface. The reason for
this is the project's core requirement: **the system must be enhanced later (hybrid
search, freshness, reranking) without rewrites.** That is only possible if each piece is
independently swappable.

There are two paths:

- **Ingestion path** (offline, runs when content changes): Loader → Parser → Chunker →
  Embedder → Store.
- **Query path** (online, per request): Query → Retriever(s) → Fusion → [Reranker] →
  Context Builder → Generator.

Plus **async jobs** that maintain freshness (supersession, recency/TTL, optional digest
synthesis).

> The interfaces below are **pseudocode**. Translate to Ptolemy's language (matching the
> repo's existing conventions for async, error handling, and dependency injection).
> Names are suggestions; keep them consistent across the module.

---

## Core data types

```text
Chunk {
  id:            string            # stable unique id
  content:       string            # the retrievable text
  embedding:     float[]           # vector; set by Embedder
  metadata:      map<string,any>   # source, title, tenant, etc.
  published_at:  timestamp         # when the content was published/created
  valid_from:    timestamp?        # optional validity window start
  valid_to:      timestamp?        # optional validity window end
  superseded_by: string?           # id of the chunk that replaced this one
}

RetrievedChunk extends Chunk {
  score: float                     # fused relevance score
}

Query {
  text:   string
  k:      int                      # final number of chunks to return
  as_of:  timestamp?               # point-in-time; defaults to "now"
  filters: map<string,any>?        # e.g. tenant_id, source
}
```

---

## Ingestion components

```text
interface Loader:
    # Pull raw documents from one source type (files, API, web).
    load(source) -> list<RawDocument>

interface Parser:
    # Convert a raw document (PDF/HTML/MD) into clean text + metadata.
    parse(doc: RawDocument) -> ParsedDocument   # { text, metadata, published_at }

interface Chunker:
    # Split text into retrievable passages. Start with fixed-size + overlap.
    chunk(doc: ParsedDocument) -> list<Chunk>   # ids + content + metadata set

interface Embedder:
    # Turn chunk texts into vectors. Batch for efficiency.
    embed(texts: list<string>) -> list<float[]>

interface Store:
    upsert(chunks: list<Chunk>) -> void          # insert/update by id
    get(ids: list<string>) -> list<Chunk>
    mark_superseded(old_id: string, new_id: string) -> void
    # retrieval lives behind the Retriever interface, not here
```

**Phase 0 defaults:** one Loader for the most common source; fixed-size Chunker
(~500 tokens, ~50 overlap — make these config, not constants); one Embedder; Store backed
by Postgres + `pgvector`.

---

## Retrieval components

This is the most important boundary. Hybrid search = **two retrievers behind one
interface** + a fusion step.

```text
interface Retriever:
    retrieve(query: Query, depth: int) -> list<RetrievedChunk>
    # depth = candidate count to pull BEFORE fusion (keep generous, e.g. 20-40).

interface Fusion:
    fuse(ranked_lists: list<list<RetrievedChunk>>, k: int) -> list<RetrievedChunk>
```

Implementations to build across phases:

- `VectorRetriever`  — dense/semantic search via `pgvector`. (Phase 0)
- `Bm25Retriever`    — lexical search via `pg_textsearch`. (Phase 1)
- `HybridRetriever`  — see note below. (Phase 1)
- `RrfFusion`        — Reciprocal Rank Fusion. (Phase 1)
- `PassthroughFusion`— returns its single input list unchanged. (Phase 0)

**Implementation choice for hybrid (see `RETRIEVAL.md` for the SQL):**
Start with **Option A** — `HybridRetriever` runs *one* SQL query that does both arms and
RRF inside Postgres, and exposes it behind the `Retriever` interface. This is the fast,
single-round-trip MVP. Because it is behind the interface, you can later split it into
separate `VectorRetriever` + `Bm25Retriever` + app-side `RrfFusion` (**Option B**) if you
need to inspect each arm or swap the fusion — with nothing downstream changing.

```text
interface Reranker:                  # Phase 3, optional
    rerank(query: Query, candidates: list<RetrievedChunk>) -> list<RetrievedChunk>

interface ContextBuilder:
    # Select/format final chunks into a prompt within a token budget; attach source ids.
    build(query: Query, chunks: list<RetrievedChunk>) -> PromptContext

interface Generator:
    # Call the LLM with the context; return answer + the source ids it used.
    generate(query: Query, context: PromptContext) -> { answer: string, citations: list<string> }
```

---

## Async jobs (off the query path)

```text
interface SupersessionJob:     # Phase 2
    # Detect when a new chunk replaces an older one; call Store.mark_superseded.
    run() -> void

interface RecencyTtlJob:       # Phase 2
    # Refresh/expire aging content per policy.
    run() -> void

interface DigestSynthesisJob:  # Phase 3, optional (StructMem-inspired)
    # Consolidate the latest chunks per topic into an evolving summary,
    # WITH a revision step that resolves contradictions against the prior digest.
    run() -> void
```

---

## Pipeline wiring (orchestrator)

```text
# Query path
function answer(query):
    retriever = HybridRetriever(...)            # Phase 0: VectorRetriever
    fusion    = RrfFusion()                      # Phase 0: PassthroughFusion
    candidates = retriever.retrieve(query, depth=config.depth)
    fused      = fusion.fuse([candidates], k=config.final_k)
    # fused     = reranker.rerank(query, fused)[:query.k]   # Phase 3
    context    = context_builder.build(query, fused)
    return generator.generate(query, context)
```

The orchestrator must read components from config/DI so swapping an implementation is a
configuration change, not a code edit. **Do not** hard-code the retriever or fusion.

---

## Design rules (do not violate)

1. Vector and BM25 indexes must cover the **same chunk rows** so the RRF join aligns.
2. All user input into SQL is **parameterized** — never string-interpolated.
3. **Do not "fix" poor results by raising top-k.** Retrieval quality plateaus around
   ~60 entries; gains come from better retrieval/fusion/reranking, not more chunks.
4. Heavy work (embedding, synthesis) is **asynchronous**; the query path stays light.
5. Each phase must keep prior phases' acceptance tests green.
