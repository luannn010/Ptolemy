---
project: Ptolemy
category: core runtime
status: Ready
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Core Runtime

## What this aspect means

The core runtime is the worker daemon, session model, command executor, file and Git surfaces, runtime storage, and the main HTTP API that everything else builds on.

## Current implementation status

Status: `Ready`

`cmd/workerd` composes configuration, SQLite stores, tmux execution, the agent loop, and Pack Studio. `internal/httpapi/router.go` exposes health, execute, sessions, commands, file ops, KB, Git, worktree, task, and UI routes.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Worker bootstrap | `cmd/workerd/main.go` | Shows runtime composition and startup |
| Router | `internal/httpapi/router.go` | Defines the main API surface |
| Execute path | `internal/executor/executor.go` | Runs commands and stores results |
| Session persistence | `internal/session/store.go` | Creates, lists, gets, closes sessions |
| SQLite base + migrations | `internal/store/store.go`, `internal/store/migrations.go` | Persistent state is implemented |

## Ready-to-use capabilities

- HTTP health, session, command, file, KB, Git, worktree, and task endpoints
- SQLite-backed session and command history
- tmux-backed command execution with fallback runner
- Pack Studio mounting under `/ui`

## Partial or unfinished capabilities

- Runtime validation from this thread is incomplete because `go test ./...` could not run here
- Production deployment automation is thin

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| HTTP daemon | `cmd/workerd` | Starts the worker API | Ready |
| Health API | `GET /health` | Service health | Ready |
| Execute API | `POST /execute` | Command execution | Ready |
| Session API | `/sessions` | Session lifecycle | Ready |

## Important commands

```bash
go run ./cmd/workerd
curl -s http://localhost:8080/health
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `cmd/workerd/main_test.go` | Router wiring and bootstrapped runtime assumptions | Test file present |
| `internal/httpapi/router_test.go` | Core route behavior | Test file present |
| `go test ./...` | Repo-wide runtime validation | Could not run here because `go` was unavailable |

## Risks

- Runtime readiness is stronger in code than in environment validation from this thread.
- The tmux path is Unix-centric and relies on local environment assumptions.

## Gaps

- No verified CI result in this audit.
- No containerized deployment path was found.

## Next recommended improvements

- Add CI-backed runtime validation.
- Document the canonical runtime boot sequence with example payloads.

