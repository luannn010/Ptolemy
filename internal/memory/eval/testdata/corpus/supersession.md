---
published_at: 2024-03-01T00:00:00Z
---
# Supersession

A new chunk replaces an older one for the same fact by setting
`superseded_by = <new_id>` on the old row. The retrieval SQL filters
`WHERE superseded_by IS NULL`, so stale facts stop being retrieved
without being deleted. Detection is explicit: the ingest caller passes
Metadata["supersedes"] = "<old-doc-id>".
