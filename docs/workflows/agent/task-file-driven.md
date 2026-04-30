# Task-File Driven Workflow

Use structured instructions instead of free-form prompts.

```text
Agent
  -> Ensures task lifecycle folders exist
  -> Can preview inbox execution order with the plan CLI
  -> Selects exactly one task by queue priority
  -> Classifies the selected task
  -> Moves executable tasks through active/process
  -> Splits large inbox/active tasks into docs/tasks/split
  -> Runs ptolemy-agent on exactly one process task
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

Queue priority:

1. `docs/tasks/process`
2. `docs/tasks/active`
3. `docs/tasks/split`
4. `docs/tasks/inbox`

Task outcomes:

- `split`: large inbox/active task creates split child tasks and archives the parent.
- `completed`: task moves from process to done and is copied to archive.
- `failed`: task moves from process to failed and writes a notification.

Artifacts:

- command logs are written to `.state/task-runner/*-output.txt`
- failure notifications are written to `.state/task-runner/notifications`

Status: working for deterministic one-task-per-run execution; task-file decomposition is simple bullet/paragraph splitting.

Related commands:

```bash
go run ./cmd/ptolemy-task-runner
go run ./cmd/ptolemy-task-runner plan --inbox docs/tasks/inbox
go run ./cmd/ptolemy-task-runner run --inbox docs/tasks/inbox --workspace .
```

Notes:

- the default command uses the queue-driven one-task-at-a-time workflow above
- `plan` previews deterministic task order from metadata without running validations
- `run` uses the sequential scheduler to validate inbox tasks and update their statuses
