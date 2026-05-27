---
published_at: 2026-01-01T00:00:00Z
---
# RAG_TOP_K default (v2)

The retrieval orchestrator defaults RAG_TOP_K to 8 if the env var is
unset (revised from 5 in v1 after the Phase 1 eval set showed n=5
truncated some paraphrase-class hits). Per-query override via Query.K
still applies.
