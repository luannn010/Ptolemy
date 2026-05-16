---
project: Ptolemy
category: workflows
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - workflows
---

# Failure and Resume Flow

The current failure-and-resume story combines workflow docs and runtime state:

1. For worker EOF or drop conditions, use `docs/workflows/recovery/eof-worker-drop.md`.
2. For invalid multi-object model replies, use `docs/workflows/recovery/invalid-multi-action.md`.
3. Agent runs can be resumed through `POST /agent-runs/{id}/resume`.
4. Pack Studio marks interrupted running program runs failed on service recovery.

Evidence:

- `docs/workflows/recovery/eof-worker-drop.md`
- `docs/workflows/recovery/invalid-multi-action.md`
- `internal/httpapi/agent_runs.go`
- `internal/packstudio/service.go`
- `cmd/ptolemy-agent/recovery_test.go`

