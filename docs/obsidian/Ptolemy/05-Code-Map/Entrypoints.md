---
project: Ptolemy
category: code-map
status: Ready
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - code-map
---

# Entrypoints

## workerd

Path: `cmd/workerd`

Purpose:

Runs the HTTP worker daemon, initializes stores, agent loop, runner, and Pack Studio.

How to run:

```bash
go run ./cmd/workerd
```

Status:

Ready

Evidence:

`cmd/workerd/main.go`

## ptolemy-mcp

Path: `cmd/ptolemy-mcp`

Purpose:

Runs the MCP stdio adapter for worker-backed tools.

How to run:

```bash
go run ./cmd/ptolemy-mcp
```

Status:

Ready

Evidence:

`cmd/ptolemy-mcp/main.go`

## ptolemy-agent

Path: `cmd/ptolemy-agent`

Purpose:

Prototype local LLM-driven executor that talks to the brain and worker services.

How to run:

```bash
go run ./cmd/ptolemy-agent --task-file docs/tasks/inbox/<task>.md --max-steps 8
```

Status:

Prototype

Evidence:

`cmd/ptolemy-agent/main.go`

## ptolemy-task-runner

Path: `cmd/ptolemy-task-runner`

Purpose:

Plans, runs, and bootstraps inbox tasks and task packs.

How to run:

```bash
go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/templates/task-pack-template
go run ./cmd/ptolemy-task-runner run --pack docs/tasks/templates/task-pack-template --workspace .
go run ./cmd/ptolemy-task-runner bootstrap --workspace /path/to/target
```

Status:

Partial

Evidence:

`cmd/ptolemy-task-runner/main.go`

