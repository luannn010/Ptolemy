---
published_at: 2024-02-01T00:00:00Z
---
# RAG_TOP_K default (v1)

The retrieval orchestrator defaults RAG_TOP_K to 5 if the env var is
unset. The default applies only at MemoryConfig load; downstream
callers may override per query via Query.K.
