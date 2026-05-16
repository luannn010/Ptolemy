---
project: Ptolemy
category: task packs and workflow engine
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Task Packs and Workflow Engine

## What this aspect means

This covers task-file validation, inbox scanning, planning, pack parsing, program orchestration, large-task splitting, and the sequential runtime that moves work from planning into execution.

## Current implementation status

Status: `Partial`

The repo contains a real workflow engine: task scanning, validation, scheduling, pack manifest parsing, program runs, process manifests, and task-runner CLI commands all exist. The engine is still sequential-first and not fully verified live here.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Task-runner CLI | `cmd/ptolemy-task-runner/main.go` | `plan`, `run`, `bootstrap`, legacy default run |
| Task-pack loader | `internal/tasks/pack.go` | Manifest parsing and pack validation |
| Task scheduler/runtime | `internal/tasks/scheduler.go`, `internal/tasks/runner.go`, `internal/tasks/process.go`, `internal/tasks/pack_runtime.go` | Execution engine |
| Pack Studio program orchestration | `internal/packstudio/service.go` | Program and run state |
| Workflow docs | `docs/workflows/agent/task-file-driven.md`, `docs/workflows/agent/task-pack-execution.md` | Intended operating model |

## Ready-to-use capabilities

- Scan and validate inbox tasks
- Build plan previews for inboxes and packs
- Parse and run pack definitions
- Record program, pack, and pack-task run state

## Partial or unfinished capabilities

- Execution mode is explicitly `sequential_first`
- True parallel or distributed execution was not verified
- Default `runInbox` HTTP executor path uses `http.ErrNotSupported` placeholder execution

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| CLI planner | `go run ./cmd/ptolemy-task-runner plan ...` | Plan tasks or packs | Ready |
| CLI runner | `go run ./cmd/ptolemy-task-runner run ...` | Run tasks or packs | Partial |
| Bootstrap CLI | `go run ./cmd/ptolemy-task-runner bootstrap ...` | Workspace scaffolding | Partial |
| HTTP inbox run | `POST /tasks/run-inbox` | Batch inbox execution scaffold | Prototype |

## Important commands

```bash
go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/templates/task-pack-template
go run ./cmd/ptolemy-task-runner run --pack docs/tasks/templates/task-pack-template --workspace .
go run ./cmd/ptolemy-task-runner bootstrap --workspace /path/to/target
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `cmd/ptolemy-task-runner/*_test.go` | CLI and workflow behavior | Test files present |
| `internal/tasks/*_test.go` | Scanner, scheduler, pack, state, validator, process behavior | Test files present |
| `go test ./...` | Repo-wide validation | Could not run here because `go` was unavailable |

## Risks

- Placeholder execution path in `/tasks/run-inbox` can be mistaken for a full runner.

## Gaps

- No validated parallel execution.
- No live proof in this audit that pack runs complete end to end.

## Next recommended improvements

- Add end-to-end pack run smoke tests.
- Mark prototype-only surfaces more explicitly in docs and responses.

