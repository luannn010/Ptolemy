---
published_at: 2024-01-15T00:00:00Z
---
# HNSW index

The dense vector index uses pgvector's HNSW (Hierarchical Navigable
Small World) with `vector_cosine_ops`. The distance operator `<=>`
returns cosine distance (smaller = closer). HNSW is approximate but
fast at retrieval-time; build cost is paid once at insert.
