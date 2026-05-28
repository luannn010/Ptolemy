---
published_at: 2024-01-15T00:00:00Z
---
# BM25 operator

ParadeDB's pg_search exposes the `@@@` operator for BM25 match. The
hybrid retriever's BM25 CTE uses `WHERE content @@@ $1` and orders
by `paradedb.score(id) DESC`. The operator is exact-token-friendly
and case-insensitive by default.
