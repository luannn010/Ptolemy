---
project: Ptolemy
category: observability and audit
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Observability and Audit

## What this aspect means

Observability and audit include logs, action records, approvals, command history, run events, artifacts, and inspection endpoints or streams that help operators understand what happened.

## Current implementation status

Status: `Partial`

Ptolemy persists actions, logs, approvals, command logs, agent observations, and program/pack run events in SQLite. It also writes agent artifacts to `.state` and exposes Pack Studio event and terminal streams. This is meaningful audit infrastructure, though not yet fully hardened.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Audit tables | `internal/store/migrations.go` | `actions`, `logs`, `approvals`, `run_events`, `agent_observations` |
| Command log persistence | `internal/command/store.go` | Stores command history |
| Logging store | `internal/logging/store.go` | Persists logs |
| Pack Studio event/terminal routes | `internal/httpapi/packstudio.go` | Operator monitoring surface |
| Agent artifact paths | `cmd/ptolemy-agent/main.go`, `cmd/workerd/main.go` | `.state/agent-artifacts` and `.state/agent-runs` usage |

## Ready-to-use capabilities

- SQLite-backed audit records for commands and actions
- Program-run event history
- Terminal snapshot and stream endpoints

## Partial or unfinished capabilities

- No dedicated analytics or metrics subsystem found
- No evidence of retention policies beyond conventions

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| Command list | `GET /sessions/{id}/commands` | View command history | Ready |
| Agent action list | `GET /agent-runs/{id}/actions` | Inspect agent actions | Ready |
| Agent observation list | `GET /agent-runs/{id}/observations` | Inspect observations | Ready |
| Pack Studio events | `GET /ui/api/program-runs/{id}/events` | Run events | Partial |

## Important commands

```bash
tree .state/agent-artifacts
cat state/workerd.out.log
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/logging/logging_test.go` | Log store behavior | Test file present |
| `internal/action/store_test.go` | Action store behavior | Test file present |
| `internal/command/store_test.go` | Command log behavior | Test file present |

## Risks

- Observability is mostly runtime-state oriented, not ops-metrics oriented.

## Gaps

- No structured tracing or metrics backend.
- No documented retention/rotation workflow.

## Next recommended improvements

- Add operator docs for interpreting event and artifact records.
- Add retention and cleanup policy docs for `.state` and runtime tables.

