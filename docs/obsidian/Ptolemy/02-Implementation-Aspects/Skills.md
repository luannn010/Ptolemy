---
project: Ptolemy
category: skills
status: Planned
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - implementation
---

# Skills

## What this aspect means

In Ptolemy terms, a skills layer would be a reusable library of durable capabilities exposed as explicit modules rather than only as ad hoc tool calls or workflow docs.

## Current implementation status

Status: `Planned`

I did not find an implemented `internal/skills/` package tree. The repo documents a preferred future Go-native skills architecture in `AGENTS.md`, but the current implementation uses agent tools, MCP tools, workflow docs, and task packs instead.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Future skills architecture is described, not implemented | `AGENTS.md` | Shows desired `internal/skills/...` layout |
| Current tool-based execution path | `internal/agentloop/tool_executor.go` | Capabilities exist as tools |
| Current MCP capability path | `internal/mcp/*tools/tools.go` | Externalized tool groups exist |

## Ready-to-use capabilities

- Tool execution through the agent loop
- MCP-exposed tool families

## Partial or unfinished capabilities

- No dedicated skills package architecture
- No stable skill registry beyond tools and workflows

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| Design target | `AGENTS.md` | Proposed long-term skill architecture | Planned |
| Tool registry | `internal/agentloop/tool_registry.go` | Current substitute for a skill runtime | Partial |

## Important commands

```bash
rg --files internal
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| N/A | No explicit skills subsystem found | Not applicable |

## Risks

- Architecture language may imply a skills subsystem that does not yet exist.

## Gaps

- Missing `internal/skills/` implementation.
- No documentation that distinguishes tools from future skills clearly enough.

## Next recommended improvements

- Decide whether the skills concept should be implemented or removed from current planning language.
- If implemented, define how skills differ from agent tools and MCP tools.

