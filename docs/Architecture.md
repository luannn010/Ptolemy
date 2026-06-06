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
nil (e.g. in tests) `/health` falls back to a static
`{"status":"ok","service":"workerd","timestamp":"<RFC3339>"}` (no `checks` array).

## Recall reasoning trace (`internal/memory`)

`ptolemy_memory_recall` accepts an opt-in `trace` boolean (which implies
`generate`). When set, the result carries `mode` (`agentic`|`legacy`) and a
`steps` array: one entry per recall step with the planner action, the query it
issued, the chunks it retrieved (id, score, ~120-rune snippet), and the terminal
outcome (grounding result or give-up reason). The trace is built in-memory from
data the loop already holds, is `nil`/absent by default, and never alters the
answer text or citations. Types `RecallTrace`/`TraceStep`/`TraceChunk` live in
`internal/memory/trace.go`; the trace is emitted by `AgentLoop.Run` (agentic
path) and `Orchestrator.Answer` (legacy path), and serialized to `steps` by the
`memorytools` recall handler. There is no live streaming or MCP notification —
the trace is returned in the single tool result, consistent with MCP's
request/response model.

## Capture extractor tuning (`internal/memory`)

The capture extractor is grammar-constrained and prompt-versioned. `fact_predicate`
is pinned by `grammar/atom.gbnf` to the fixed taxonomy in `validators.go`
(`allowedPredicates`) — the model cannot emit an out-of-taxonomy predicate
(`TestPredicateGrammarMatchesTaxonomy` guards the two lists against drift, mirroring
the `action.gbnf` precedent). The `extract_v2` prompt (`prompts/extract_v2.txt`,
stamped as `ExtractorVersion`) lists that vocabulary, extracts less conservatively,
and requires `content` to be copied from the source turn (trim filler, no paraphrase)
so atoms survive `EvidenceInSourceValidator`; the resolved entity goes in
`fact_subject`. No validator was changed.

## Controller worker-pool (`internal/controller`)

The first deterministic slice of the multi-agent orchestration layer. Three pure
units plus a supervisor: an in-process `Bus` (non-blocking fan-out — each
subscriber drains its own bounded channel; a full buffer drops + counts rather
than stalling the publisher), a worker lifecycle `State` machine (`Pending →
Provisioning → Running → Stage1Passed → Integrating → Merged`, with `Failed` /
`Cancelled` reachable from every non-terminal state; `CanTransition` validates
against a transition table), and a `Supervisor` that spawns N workers into git
worktrees and drives each through an injected `Runner` up to `Stage1Passed`. The
supervisor performs side effects only through a `WorktreeManager` consumer
interface satisfied by `*policy.GuardedWorktree`, so it never touches a raw
adapter — consistent with the harness rule. The registry is in-memory (no schema
change); Docker, mocks, the integration lock/Stage 2, `base_updated`
propagation, and wiring in the model are deferred to later slices, per the build
order in `docs/ptolemy-architecture.html` ("wire in the model last"). The
`Runner` interface is the seam the model-backed implementation plugs into later.
