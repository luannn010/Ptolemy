---
project: Ptolemy
category: distributed workers
status: Unknown
last_verified: 2026-05-11
source: codebase
confidence: low
tags:
  - ptolemy
  - implementation
---

# Distributed Workers

## What this aspect means

This aspect would cover multiple worker nodes, remote scheduling, cross-node routing, or a cluster-style execution model.

## Current implementation status

Status: `Unknown`

I did not find a verified distributed worker subsystem in the current codebase. The architecture is strongly local-first, and the available code points to one local worker daemon plus local worktrees.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Local-first description | `README.md` | Describes a local worker platform |
| Single worker daemon entrypoint | `cmd/workerd/main.go` | No cluster orchestration code observed |
| Local worktree isolation | `internal/worktree/worktree.go` | Isolation is repo-local |

## Ready-to-use capabilities

- Local worktree isolation

## Partial or unfinished capabilities

- None verified for distributed nodes

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| N/A | N/A | No distributed-node API verified | Unknown |

## Important commands

```bash
rg -n "worker|node|cluster|remote" internal cmd docs
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| N/A | No distributed-worker implementation verified | Not applicable |

## Risks

- Roadmap assumptions could overstate horizontal scaling support.

## Gaps

- No node registry, remote scheduler, or distributed queue was found.

## Next recommended improvements

- Either document distributed workers as explicitly out of scope, or add a design note before implementation begins.

