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

# Task Pack Execution

Task pack execution currently flows through pack parsing, task validation, program or pack run creation, sequential scheduling, and agent-run execution.

High-level flow:

1. Load `PACK_MANIFEST.yaml`, `TASK_PLAN.md`, and `README.md`.
2. Validate required folders and entrypoint.
3. Scan `inbox/` tasks.
4. Build plan or create a program run.
5. Execute tasks in sequential-first order.
6. For large tasks, use process manifest state under `.ptolemy/tasks/process/<pack-id>/`.
7. Record run state in SQLite tables such as `program_runs`, `pack_runs`, `pack_run_tasks`, and `run_events`.

Evidence:

- `internal/tasks/pack.go`
- `internal/tasks/pack_runtime.go`
- `internal/packstudio/service.go`
- `docs/workflows/agent/task-pack-execution.md`

