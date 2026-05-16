---
project: Ptolemy
category: connectors
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Connectors

## What this aspect means

Connectors are the adapters that let Ptolemy talk to other processes or protocols, such as HTTP worker clients, MCP clients, or a local LLM server.

## Current implementation status

Status: `Partial`

The codebase clearly implements a worker HTTP client, an MCP stdio adapter, and a local OpenAI-compatible brain client. I did not find broader external SaaS connectors beyond those local protocol adapters.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| MCP adapter entrypoint | `cmd/ptolemy-mcp/main.go` | stdio MCP server |
| MCP server implementation | `internal/mcp/server.go` | JSON-RPC handling |
| Worker HTTP client | `internal/worker/client.go` | Session, file, command, agent-run client |
| Brain HTTP client | `internal/brain/client.go` | Talks to local chat-completions endpoint |

## Ready-to-use capabilities

- MCP-to-worker adapter
- Worker HTTP client for local CLIs
- Local brain connector for `BRAIN_BASE_URL`

## Partial or unfinished capabilities

- No verified external connectors like GitHub, Slack, or remote execution nodes
- Brain integration appears single-provider and local-first

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| MCP stdio | `cmd/ptolemy-mcp` | Exposes worker tools over MCP | Ready |
| Worker client | `internal/worker/client.go` | Calls worker API from CLIs | Ready |
| Brain client | `internal/brain/client.go` | Calls local model server | Partial |

## Important commands

```bash
go run ./cmd/ptolemy-mcp
curl -s http://127.0.0.1:8088/v1/chat/completions
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/worker/client_test.go` | Worker client behavior | Test file present |
| `internal/brain/client_test.go` | Brain client behavior | Test file present |
| `cmd/ptolemy-mcp/main_test.go` | MCP entry wiring | Test file present |

## Risks

- Connector strategy is mostly local and may not cover remote automation needs.

## Gaps

- No generalized connector framework beyond MCP/HTTP clients.

## Next recommended improvements

- Clarify whether remote or third-party connectors are in scope.
- Add live integration smoke tests for worker and brain endpoints.

