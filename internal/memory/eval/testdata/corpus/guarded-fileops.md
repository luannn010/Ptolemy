---
published_at: 2024-02-01T00:00:00Z
---
# GuardedFileOps

Services hold Guarded* wrappers only — never raw adapters. GuardedFileOps
wraps the fileops adapter and routes every call through internal/policy
before any side effect. Raw adapters live exclusively in cmd/workerd/main.go.
