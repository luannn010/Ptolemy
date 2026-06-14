# Ptolemy

Ptolemy is a local-first agent runtime. A worker daemon (`workerd`) gives a planner
(Claude Code, Codex, or any MCP client) controlled hands on a machine — sessions,
shell commands, files, git, worktrees — while a **policy harness** sits between
intent and effect: every side-effecting call is authorized against a ruleset,
audited to SQLite, and either allowed, paused for human approval, or denied. On
top of that runtime sits a **conversational memory** system (hybrid RAG on
PostgreSQL + pgvector) with an agentic retrieval loop, exposed both as MCP tools
and as a plain HTTP `/chat` endpoint for sub-services.

This tree is the **v2 clean-room rebuild**: packages are ported from
`ptolemy-legacy/` one by one (copy + adapt + test, never import), each landing
behind the harness with its own tests and a note in
[docs/Architecture.md](docs/Architecture.md).

## The policy harness (trust root)

`internal/policy` is the heart of v2. Side-effecting adapters (`terminal`,
`fileops`, `gitops`, `worktree`) are never reachable from services directly —
only through a `Guarded*` wrapper that runs every call through
`Authorize → record to policy_decisions → allow / ask / deny`:

- **allow** — proceeds, still audited.
- **ask** — pauses: the caller gets `202 needs_confirmation` with a
  `pending_id`; a human approves out-of-band on the loopback approve listener;
  the retried call carries the `confirm_token` (which *is* the intent hash, so
  approving intent A can never authorize a different intent B).
- **deny** — refused, audited.

The fail-safe default for anything unlisted is **ask**. The committed baseline
ruleset is `DefaultRuleset()` in
[internal/policy/rules.go](internal/policy/rules.go); a host-local override can
live at `.ptolemy/policy.json` (gitignored, and itself write-protected by the
`deny-policy-write` rule). Deny rules are never loosened — see
[CLAUDE.md](CLAUDE.md) for the guardrails. The bypass test suite lives at
[internal/policy/engine_test.go](internal/policy/engine_test.go).

Two read-only carve-outs skip the harness by design: `navigator`
(knowledge-base reads) and `internal/memory` (in-process memory whose only
writes land in the memory Postgres DB).

## Network surfaces

`workerd` serves up to three listeners:

| Port | Env | Binds | Surface |
|---|---|---|---|
| 8080 | `HTTP_PORT` | all | Worker API: `GET /health` (deep readiness), `POST/GET /sessions`, `POST /sessions/{id}/commands`, `POST /execute` |
| 8081 | `APPROVE_PORT` | loopback | `POST /approve/{pending_id}` — out-of-band human approval |
| 8090 | `RAG_PORT` | all | `POST /chat` (agentic RAG for sub-services), `GET /health` |

The RAG listener appears only when memory is configured (`DATABASE_URL` etc.);
otherwise workerd logs that it's disabled and keeps serving the rest. The
approve surface is loopback-only on purpose — approving intents is an operator
action.

> ⚠️ **`RAG_PORT` (and the worker API) bind all interfaces and have no
> authentication.** `/chat` reaches the LLM and the memory DB. If the host sits
> on an untrusted network, restrict the port — firewall it, bind it behind a
> reverse proxy, or expose it only over a VPN. The body is capped (1 MiB) but
> there is no auth or rate limiting in-process.

## Agentic RAG over HTTP (`POST /chat`)

Sub-services ask questions; Ptolemy retrieves, reasons, and answers grounded in
its memory:

```bash
curl -s http://<host>:8090/chat -H "Content-Type: application/json" \
  -d '{"query":"How does the approval flow work?", "trace":true}'
```

Request: `{query, k?, subject_id?, project_id?, trace?}`. Response:
`{answer, citations, gave_up}` — plus `mode` and a step-by-step retrieval
`steps` trace when `trace:true`. `gave_up:true` is an honest 200 ("not in the
KB"); upstream failures (brain LLM / embedder / DB) map to 502. With
`AGENT_LOOP_ENABLED=true` answers come from the agentic planner + grounding
loop instead of the single-shot pipeline.

Because `memory.NewModule` hands back a single non-concurrency-safe `*pgx.Conn`,
the handler is serialized (`NewSerialAnswerer`), and the listener uses a generous
120s write timeout because an agentic answer is several LLM round-trips.

## Conversational memory (MCP)

`internal/memory` implements hybrid retrieval (dense pgvector + BM25, fused
with reciprocal-rank fusion), recency ranking, grammar-constrained capture
extraction, GC/dedup sweeps, and an agentic recall loop with reasoning traces.
It is exposed as three MCP tools — `ptolemy_memory_recall`,
`ptolemy_memory_capture`, `ptolemy_memory_consolidate` — plus the
`ptolemy-memory` CLI. Scope defaults from `PTOLEMY_MEMORY_SUBJECT` /
`PTOLEMY_MEMORY_PROJECT`. The full build spec lives under
[docs/memory/](docs/memory/README.md).

The local LLM ("brain", `BRAIN_BASE_URL`) and the embedder
(`EMBEDDING_BASE_URL`) are external endpoints this runtime talks to — generation
and embeddings for the RAG path, and a `GET /v1/models` liveness probe in
`/health`.

## Binaries

`make build` produces four:

| Binary | Purpose |
|---|---|
| `workerd` | the worker daemon (all listeners above) |
| `ptolemy-mcp` | stdio MCP adapter exposing worker + memory tools |
| `ptolemy` | CLI: `policy check`, `memory demo\|eval\|synth-eval`, `memory recall\|capture` |
| `ptolemy-memory` | thin alias for `ptolemy memory recall\|capture` (hook-friendly) |

## Build, test, configure

```bash
make build          # bin/{workerd,ptolemy-mcp,ptolemy,ptolemy-memory}
make test           # go test -p 1 ./...
make smoke-memory   # end-to-end ingest+ask against your .env
make eval-memory    # retrieval eval on the frozen fixture corpus
```

Copy [.env.example](.env.example) to `.env` and fill in what you use: SQLite
state (`DB_PATH`), memory Postgres (`DATABASE_URL`), embedder
(`EMBEDDING_BASE_URL`, `EMBEDDING_MODEL`, `EMBEDDING_DIM`), brain
(`BRAIN_BASE_URL`, `BRAIN_MODEL`), and the agentic loop (`AGENT_LOOP_ENABLED`).
Anything unset degrades gracefully — workerd logs what it disabled and keeps
serving.

Execution state is SQLite with exactly four tables (`sessions`,
`command_logs`, `policy_decisions`, `schema_migrations`); memory lives in
PostgreSQL. Go 1.25, module `github.com/luannn010/ptolemy`.

## Repository layout

```text
cmd/
  workerd/          worker daemon + listener wiring
  ptolemy-mcp/      MCP stdio adapter
  ptolemy/          CLI (policy check, memory demo/eval/recall/capture)
  ptolemy-memory/   alias binary for memory recall/capture
internal/
  policy/           THE TRUST ROOT — engine, rules, approvals, Guarded* adapters
  domain/           intents, decisions, effects
  memory/           hybrid RAG, capture/recall/consolidate, agent loop, GC
  httpapi/          routers: worker API, approvals, RAG /chat
  mcp/              MCP tool definitions + JSON-RPC server
  health/           deep /health aggregator
  controller/       multi-agent worker-pool orchestration (Stage 1/2 slices)
  config/           env-backed configuration
  command/ terminal/ shellcmd/                    command execution path (behind GuardedRunner)
  fileops/ gitops/ worktree/ workspace/ inspect/  raw adapters (behind guards)
  navigator/        read-only KB access (carve-out)
  session/ store/ logging/ apitypes/ cli/         support packages
docs/
  Architecture.md   one-paragraph note per landed package
  memory/           memory module build spec
  deploy.md
```

## Contributing & agent rules

[AGENTS.md](AGENTS.md) is authoritative for branching, commits, and PRs;
[CLAUDE.md](CLAUDE.md) overlays the Claude Code harness rules. The short
version: feature branches are `ptolemy/<task-slug>`; commits are per-phase with
explicit staging (never `git add .`); tests precede implementation for anything
touching the harness; PRs use
[.github/pull_request_template.md](.github/pull_request_template.md); README
and docs are refreshed before any dev branch merges to `main`; and nothing is
pushed without explicit approval.
