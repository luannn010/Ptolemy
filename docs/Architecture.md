# Ptolemy v2 — Architecture notes

Per the Definition of Done in [CLAUDE.md](../CLAUDE.md), each ported or newly-added
package carries a one-paragraph note here. This file is the running index; it will
grow as packages land. (The full restored bootstrap architecture lives alongside in
`docs/ptolemy-architecture.html`.)

## Health endpoint (`internal/health`)

workerd's `GET /health` is a deep readiness probe. The `internal/health` package
defines a `Checker` interface and an `Aggregator` that runs one checker per
dependency in parallel under a per-check timeout (`HEALTH_TIMEOUT_MS`, default
1500ms). Brain and Embedder are probed with `GET /v1/models`; the Workerd line
pings its own SQLite store; Postgres (memory DB) is pinged via a lazily-opened
`pgxpool`; MCP is probed with `GET /health`. Brain, Embedder, and Workerd are
required — any one down yields overall `unhealthy` and HTTP 503. Postgres and MCP
are optional — down yields `degraded` (200) and an unset endpoint yields `disabled`
(200). All checks are read-only probes that touch no workspace, shell, or git, so
the package needs no `Guarded*` wrapper, consistent with the harness rules. The
checkers and the optional Postgres pool are constructed in `cmd/workerd/main.go`
and injected into the router via `httpapi.RouterDeps.Health`; when that field is
nil (e.g. in tests) `/health` falls back to a static `{"status":"ok"}`.
