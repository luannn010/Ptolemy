# Task Pack Execution Workflow

Use this workflow when the user is creating or running a task pack through Pack Studio or the task-runner CLI.

## Goal

The normal happy path should now be:

```text
Author task pack
  -> create pack metadata and inbox tasks
  -> start a pack or program run
  -> let Ptolemy split large tasks into child tasks when needed
  -> execute each child task with fresh model context
  -> persist manifest, todo, and child summaries
  -> monitor progress from Pack Studio Runs
```

## Authoring Rules

- Prefer task packs for multi-step work.
- Keep each inbox task narrow when possible.
- If a task is large, the runtime may split it into child tasks automatically.
- Child tasks should stay in one phase only: `inspect`, `plan`, `edit`, `test`, `validate`, `docs`, or `finalize`.
- Comfortable child task body size is roughly `1200-2000` chars.
- Above `4000` chars is risky and should usually be split further.

## Runtime Behavior

For a large inbox task, Ptolemy now uses a manifest-driven process flow instead of carrying one long chat history:

```text
Parent task
  -> classify as large
  -> split deterministically into child tasks
  -> create .ptolemy/tasks/process/<pack-id>/manifest.json
  -> create .ptolemy/tasks/process/<pack-id>/todo.md
  -> run child 001 with fresh context
  -> save compact result summary
  -> update manifest
  -> reset context
  -> run child 002
  -> repeat until done or failed
```

Each child task gets:

- the fixed system prompt
- the current child task only
- the relevant manifest slice
- compact previous child summaries
- budgeted `.ptolemy` KB/context snippets

Each child task does not get:

- the whole parent pack body
- full prior child logs
- full raw observation history from earlier child tasks

## Process State Files

Execution state is written under:

```text
.ptolemy/tasks/process/<pack-id>/
├── manifest.json
├── todo.md
└── state/
    ├── 001-...-result.md
    ├── 002-...-result.md
    └── ...
```

Use these files as the durable memory for multi-step execution.

## Monitoring

Use Pack Studio under `/ui`:

- `Studio` to author packs and programs
- `Overview` to inspect catalog and run history
- `Runs` to monitor Program -> Pack -> Task -> Agent Run progress
- `Runs` live terminal stream to inspect current execution output

When debugging a run, prefer a fresh rerun after fixing the blocker rather than interpreting stale failed-run terminal state.

## Commands

```bash
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

## Notes

- Small tasks run directly.
- Medium tasks run directly with budgeted context.
- Large tasks are split before or during execution using deterministic rules.
- The agent still returns exactly one JSON action at a time.
