# Task Pack Runner Workflow

Use this workflow when you want to understand what happens today when Ptolemy runs a task pack.

Status: partially implemented.

## Purpose

A task pack groups related tasks and shared assets in one directory:

- `TASK_PLAN.md`
- `PACK_MANIFEST.yaml`
- `README.md`
- `scripts/`
- `task-scripts/`
- `snippets/`
- `inbox/`

The current runner can load the pack, validate its structure, and run the pack's inbox tasks in dependency order.

## Current Implemented Flow

```text
Task pack directory
  -> Load TASK_PLAN.md, PACK_MANIFEST.yaml, and README.md
  -> Validate required folders exist
  -> Scan pack inbox tasks
  -> Validate task metadata
  -> Validate referenced task-scripts and snippets exist
  -> Build deterministic dependency order
  -> Run each task's validation commands sequentially
  -> Update each task status to running/completed/failed
  -> Stop on first failed task
```

Current entrypoints:

```bash
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

## What Is Implemented Today

- Pack manifest parsing through `internal/tasks/pack.go`
- Pack structure validation for `TASK_PLAN.md`, `PACK_MANIFEST.yaml`, `README.md`, and required folders
- Task parsing and validation through `internal/tasks/task.go` and `internal/tasks/validator.go`
- Deterministic dependency planning
- Sequential execution through `RunTaskPack(...)`
- Task status file updates to `running`, `completed`, or `failed`

## What Is Not Implemented Yet

The current task-pack runner does not yet:

- create a git branch for the pack or for each task
- check out a worktree for isolated execution
- read a snippet and automatically apply code edits from it
- execute `task-scripts/` files automatically
- execute pack `scripts/` hooks automatically
- raise a GitHub issue when a task or pack fails
- create, push, or open a pull request to merge back to `main`

Important detail:

- the `branch` field is currently required metadata on each task, but the runner validates its presence only; it does not create or switch branches
- `snippets/` and `task-scripts/` are currently validated references only

## Logging And Failure Behavior

There are two different flows in the repository today:

1. `go run ./cmd/ptolemy-task-runner`
   This queue-driven mode writes logs to `.state/task-runner/*-output.txt` and writes failure notifications to `.state/task-runner/notifications`.

2. `go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .`
   This pack runner path currently returns scheduler results and updates task status, but it does not yet save pack-level logs or create GitHub issues.

## Recommended Future Target Flow

If you want full task-pack automation, the intended future flow should look like this:

```text
Load pack
  -> Validate manifest and tasks
  -> Create branch or worktree for runnable task
  -> Run ptolemy-agent for the task
  -> Save logs and artifacts
  -> On failure, write local notification and optionally raise GitHub issue
  -> On success, run validations
  -> Commit task-scoped changes
  -> Push branch with approval
  -> Open PR against main
```

Until that exists, task packs should be treated as a validated sequential task list, not as a full GitHub automation pipeline.
