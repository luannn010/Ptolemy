---
project: Ptolemy
category: feature-matrix
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - ready
---

# Ready To Use

These are the strongest currently implemented surfaces based on code evidence.

## Runtime and API

- `workerd` startup and routing in `cmd/workerd/main.go`
- Session lifecycle in `internal/httpapi/session.go`
- Command execution in `internal/executor/executor.go`
- File operations in `internal/httpapi/fileops.go`
- Git operations in `internal/httpapi/git.go`

## Tooling and adapters

- MCP server in `cmd/ptolemy-mcp/main.go`
- MCP tool families in `internal/mcp/*tools/tools.go`
- Agent tool registry in `internal/agentloop/tool_executor.go`

## State and knowledge

- SQLite runtime persistence in `internal/store/store.go` and `internal/store/migrations.go`
- KB build/read/update in `internal/navigator/kb.go`

## Task execution foundations

- Task scanning and validation in `internal/tasks/scanner.go` and `internal/tasks/validator.go`
- Pack manifest parsing in `internal/tasks/pack.go`

Related notes:

- [[Feature Inventory]]
- [[../02-Implementation-Aspects/Core Runtime]]
- [[../02-Implementation-Aspects/Tools]]

