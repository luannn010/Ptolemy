---
project: Ptolemy
category: decisions
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - adr
---

# ADR-0001 Current Architecture

## Status

Accepted as the current observed architecture.

## Context

The codebase has grown from a local worker and CLI executor into a broader platform with HTTP, MCP, KB, task-pack, and Pack Studio surfaces. The main risk is architectural drift between prototype and canonical paths.

## Decision

Document the current architecture as:

- `workerd` is the central runtime process.
- SQLite is the canonical execution state store.
- `.ptolemy/kb` is the canonical knowledge base layer.
- MCP is the main external adapter protocol.
- `internal/agentloop` is the emerging canonical agent runtime.
- `cmd/ptolemy-agent` remains a prototype or compatibility path until explicitly elevated.

## Consequences

- Documentation should prefer `workerd` + `/agent-runs` when describing the main future runtime.
- Prototype surfaces should be labeled clearly.
- Future changes should reduce duplicate execution paths where possible.

