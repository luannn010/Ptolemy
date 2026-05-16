# Task-File Driven Workflow

Use structured instructions instead of free-form prompts.

Task files that participate in this workflow should also follow `docs/workflows/agent/task-flags-and-isolation.md`, and new task files should use the templates in `docs/tasks/templates/`.

```text
Agent
  -> Ensures task lifecycle folders exist
  -> Can preview loose-task or task-pack execution order with the plan CLI
  -> Selects exactly one task by queue priority
  -> Classifies the selected task
  -> Moves executable tasks through active/process
  -> Runs small or medium tasks directly
  -> Splits large work into child-task processes when needed
  -> Runs ptolemy-agent on exactly one direct task or child task at a time
  -> Moves completed tasks to done and archives a copy
  -> Moves failed tasks to failed and writes a notification
```

Current task runner paths:

- `docs/tasks/inbox`
- `docs/tasks/active`
- `docs/tasks/process`
- `docs/tasks/split`
- `docs/tasks/done`
- `docs/tasks/failed`
- `docs/tasks/archive`

Task-pack layout:

- `<pack>/TASK_PLAN.md`
- `<pack>/PACK_MANIFEST.yaml`
- `<pack>/README.md`
- `<pack>/scripts`
- `<pack>/task-scripts`
- `<pack>/snippets`
- `<pack>/inbox`

Queue priority:

1. `docs/tasks/process`
2. `docs/tasks/active`
3. `docs/tasks/split`
4. `docs/tasks/inbox`

Task outcomes:

- `split`: legacy queue split behavior for large inbox/active tasks.
- `process`: large task creates a process manifest and child-task state under `.ptolemy/tasks/process/<pack-id>/`.
- `completed`: task moves from process to done and is copied to archive.
- `failed`: task moves from process to failed and writes a notification.

Artifacts:

- command logs are written to `.state/task-runner/*-output.txt`
- failure notifications are written to `.state/task-runner/notifications`
- process manifests, todos, and child summaries are written to `.ptolemy/tasks/process/<pack-id>/`

Status: working for deterministic one-task-per-run execution with fresh-context child-task processes for large tasks.

Related commands:

```bash
go run ./cmd/ptolemy-task-runner
go run ./cmd/ptolemy-task-runner plan --inbox docs/tasks/inbox
go run ./cmd/ptolemy-task-runner run --inbox docs/tasks/inbox --workspace .
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

Notes:

- the default command uses the queue-driven one-task-at-a-time workflow above
- `plan` previews deterministic task order from metadata without running validations
- `run` uses the sequential scheduler to validate task metadata and update task statuses
- task packs are executed directly from the pack directory in v1; they are not copied into `docs/tasks/inbox` first
- large tasks can be split into child tasks automatically during execution
- child tasks carry forward compact summaries, not the whole raw chat history
- pack `task-scripts/` and `snippets/` are validated references only in v1, and pack `scripts/` hooks are not auto-run
