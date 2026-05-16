---
project: Ptolemy
category: ui
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - implementation
---

# UI

## What this aspect means

This covers human-facing interfaces beyond raw CLI and API use, including the embedded web UI and any TUI or dashboard layers.

## Current implementation status

Status: `Partial`

An embedded Pack Studio web UI exists under `/ui` with overview, pack, program, run, event stream, and terminal stream routes. I did not find a separate TUI. The UI is implemented but still reads as an early product surface rather than a fully hardened dashboard.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Embedded UI routes | `internal/httpapi/packstudio.go` | `/ui` and `/ui/api/*` routes |
| Static assets | `internal/httpapi/ui/index.html`, `internal/httpapi/ui/app.js`, `internal/httpapi/ui/styles.css` | Frontend implementation exists |
| Pack Studio service | `internal/packstudio/service.go` | Provides UI data and orchestration |
| UI mention in task docs | `docs/tasks/README.md` | Describes Studio, Overview, Runs views |

## Ready-to-use capabilities

- Pack catalog and creation APIs
- Program catalog and creation APIs
- Program run detail, run events, and terminal snapshot/stream endpoints

## Partial or unfinished capabilities

- No evidence of a separate desktop app, TUI, or production web dashboard stack
- No live UI validation was performed in this audit

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| UI shell | `GET /ui`, `GET /ui/studio`, `GET /ui/overview`, `GET /ui/runs/{id}` | Embedded web shell | Partial |
| Pack Studio API | `/ui/api/*` | Pack/program/run operations | Partial |
| Terminal stream | `GET /ui/api/program-runs/{id}/terminal/stream` | Live terminal output | Partial |

## Important commands

```bash
go run ./cmd/workerd
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/httpapi/packstudio_test.go` | Pack Studio handler behavior | Test file present |
| `go test ./...` | UI-adjacent backend validation | Could not run here because `go` was unavailable |

## Risks

- UI maturity may be overstated without a live run.

## Gaps

- No TUI found.
- No screenshots or end-to-end UI test evidence in this audit.

## Next recommended improvements

- Add browser-level smoke tests for `/ui`.
- Add a dedicated UI architecture note once the surface stabilizes.

