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

## Controller Stage-2 integration (`internal/controller`)

Slice 2 of the orchestration layer adds serial promotion on top of the slice-1
supervisor. After a worker reaches `Stage1Passed`, `driveWorker` (when the
integration deps are configured) calls `integrate`: acquire a single
`IntegrationLock`, transition `Integrating`, run the injected `Stage2Runner`
against the real environment, and on success `MergeNoFF` the worker's branch to
base via a `GitMerger`, transition `Merged`, and publish `base_updated` with the
new base SHA. Stage-1 stays parallel (`MaxWorkers`); only the integration section
is serial, enforced by the lock, and `s.mu` is held only per-transition so the
long-running integration steps don't serialize unrelated workers. The production
lock (`PgLock`) is a Postgres session-level advisory lock — crash reclaim is
automatic because the lock dies with its connection — and needs no `Guarded*`
wrapper, consistent with the `internal/health` precedent for non-workspace
Postgres access (no table is added; the four-table schema is intact). The actual
code change, the merge, *does* go through `*policy.GuardedGit`. `New` panics if
the integration deps are only partially set (a wiring bug); all-nil preserves
slice-1 behavior (stop at `Stage1Passed`). The `base_updated` subscribers
(propagation, relevance), the current-with-base entry ticket, and the
Stage-2-fail regression loop are deferred to later slices.

## RAG HTTP listener (`internal/httpapi/rag.go`, `cmd/workerd`)

workerd's third listener exposes the memory module's agentic RAG to local
sub-services as plain HTTP: `POST :RAG_PORT/chat` (default 8090, all interfaces
so LAN + WSL callers can reach it) takes `{query, k?, subject_id?, project_id?,
trace?}` and returns the grounded answer, citations, `gave_up`, and — when
`trace` is set — the `mode` + `steps` reasoning trace (reusing `memory.TraceStep`).
A static `GET /health` gives sub-services a liveness probe. Wiring mirrors
`cmd/ptolemy-mcp`: `buildRAGDeps` (cmd/workerd/memory.go) loads `memory.LoadConfig`
+ `NewModule` and gracefully disables the listener (warn log, workerd still
serves) when memory is unconfigured; the agent loop engages via
`AGENT_LOOP_ENABLED`, and subject/project default from `PTOLEMY_MEMORY_*`.
Memory stays in its in-process carve-out — the endpoint is read-mostly (its only
writes are reinforce counters in the memory Postgres DB) so no `Guarded*`
wrapper is involved. Because `NewModule` returns a single non-concurrency-safe
`*pgx.Conn`, the handler goes through `NewSerialAnswerer` (mutex). gave-ups are
HTTP 200; upstream failures (brain/embedder/DB) map to 502; the server uses a
120s WriteTimeout (multi-step agentic answers) and shuts down before the memory
conn closes.

## Brain lifecycle skill (`internal/brain`, `internal/policy/guard_brain.go`, `cmd/workerd`)

workerd can manage the local llama.cpp "brain" process — start, stop, switch
model, **auto-wake on a `/chat` request**, and unload after idle (the JIT VRAM
"hibernate" pattern). Because spawning/killing a process is a side effect, it is
reached only through `policy.GuardedBrain`, never a raw exec: every op is
`Authorize`d and audited to `policy_decisions` like `GuardedRunner`. The raw
mechanism is `brain.Manager` (injected `Launcher`/`Probe` make it unit-testable
without a real process; `wake` polls `GET /v1/models` for readiness, `switch`
validates the target model before stopping the current one). Policy posture:
**automatic** actions (`brain wake`, `brain autounload`, `brain status`) are
`allow` so they don't block, while **manual destructive** actions (`brain
switch`, `brain stop`) are `ask`/OOB — confirmed via the existing
`127.0.0.1:APPROVE_PORT/approve/{id}` flow. The control plane
(`POST /brain/{wake,stop,switch}`, `GET /brain/status`) is **loopback-only**
(it can stop GPU processes); auto-wake is injected into `/chat` as an optional
`Waker`. The idle-TTL loop reads `Manager.Status` (ungated) and calls the gated
`Unload` only when actually idle, so the audit records one row per real unload,
not one per tick. Brain decisions audit under a reserved `brain-system` session
row (ensured at startup — no schema change). The whole skill is **off by
default** (`BRAIN_CONTROL_ENABLED`); model paths come from a `BRAIN_MODELS` JSON
registry. It assumes workerd is co-located with the brain (same host); the model
is wired last per the build order.
