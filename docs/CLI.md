# CLI Guide

## Main Commands

### Worker

```bash
go run ./cmd/workerd
```

### MCP Adapter

```bash
go run ./cmd/ptolemy-mcp
```

### Agent

Run a task file:

```bash
go run ./cmd/ptolemy-agent --task-file docs/tasks/<task>.md --max-steps 8
```

Allow script creation or execution only when the task explicitly needs it:

```bash
go run ./cmd/ptolemy-agent --allow-scripts --task-file docs/tasks/<task>.md --max-steps 3
```

### Task Runner

Run the queue-driven task runner:

```bash
go run ./cmd/ptolemy-task-runner
```

Preview loose inbox execution order:

```bash
go run ./cmd/ptolemy-task-runner plan --inbox docs/tasks/inbox
```

Run loose inbox tasks sequentially:

```bash
go run ./cmd/ptolemy-task-runner run --inbox docs/tasks/inbox --workspace .
```

Preview a task pack:

```bash
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
```

Run a task pack directly:

```bash
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

Example:

```bash
go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/templates/task-pack-template
go run ./cmd/ptolemy-task-runner run --pack docs/tasks/templates/task-pack-template --workspace .
```

## Typical Checks

```bash
go test ./...
curl -s http://localhost:8080/health | jq
```

## Related Reading

- [Setup](./Setup.md)
- [Task System Overview](./tasks/README.md)
- [Task-File Driven Workflow](./workflows/agent/task-file-driven.md)
