---
project: Ptolemy
category: knowledge base and memory
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Knowledge Base and Memory

## What this aspect means

This aspect covers project memory, KB generation, compatibility context files, and the read/update flow agents use before scanning the full repo.

## Current implementation status

Status: `Partial`

Ptolemy has a real KB-first implementation: it can build and update `.ptolemy/kb`, maintain file and symbol indexes, and load workspace memory from KB or compatibility context files. This is substantial, but still not fully verified live in this audit.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| KB build/update logic | `internal/navigator/kb.go` | Builds file/symbol indexes and updates KB |
| Memory loader | `internal/memory/loader.go` | Prefers `.ptolemy/kb`, then compatibility context |
| KB endpoints | `internal/httpapi/router.go` | `/kb/build`, `/kb/read`, `/kb/update` |
| KB assets exist | `.ptolemy/kb/*` | Current repository already carries KB files |

## Ready-to-use capabilities

- Build or refresh KB artifacts
- Read KB-first workspace context
- Update KB incrementally for changed files and pack metadata

## Partial or unfinished capabilities

- Compatibility-era navigator surfaces still coexist
- No proof from this thread that KB update hooks completed successfully in a live run

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| HTTP KB build | `POST /kb/build` | Build KB | Ready |
| HTTP KB read | `POST /kb/read` | Read KB context | Ready |
| HTTP KB update | `POST /kb/update` | Incremental KB update | Ready |
| MCP KB tools | `ptolemy_kb_build`, `ptolemy_kb_read`, `ptolemy_kb_update` | External KB operations | Ready |

## Important commands

```bash
curl -X POST http://localhost:8080/kb/build
curl -X POST http://localhost:8080/kb/read
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/memory/loader_test.go` | Memory loading | Test file present |
| `internal/navigator/navigator_test.go` | Navigator behavior | Test file present |
| `internal/navigator/context_budget_test.go` | Context budgeting | Test file present |

## Risks

- Compatibility and canonical KB paths can confuse callers.

## Gaps

- Missing a live, recorded KB refresh validation in this audit.

## Next recommended improvements

- Remove or clearly label compatibility-only navigator surfaces.
- Add documentation for when to use KB read vs full repo search.

