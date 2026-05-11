---
project: Ptolemy
category: architecture
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - architecture
---

# System Overview

Ptolemy currently centers on `workerd`, an HTTP daemon that coordinates sessions, command execution, file operations, Git helpers, worktrees, KB navigation, agent runs, and Pack Studio surfaces. `ptolemy-mcp` exposes much of that runtime over MCP stdio, while `ptolemy-task-runner` and `ptolemy-agent` provide CLI-facing orchestration and prototype agent behavior.

The storage model is split between SQLite runtime state and file-based knowledge state. SQLite captures sessions, command logs, actions, logs, approvals, agent runs, program runs, pack runs, and run events. `.ptolemy/kb` carries the project memory layer with `PROJECT_MAP.md`, `FILE_INDEX.json`, `SYMBOL_INDEX.json`, and related markdown.

Key evidence:

| Evidence | Path | Notes |
|---|---|---|
| Workerd wires stores, agent loop, Pack Studio, and router | `cmd/workerd/main.go` | Main runtime composition |
| HTTP surface definition | `internal/httpapi/router.go` | Shows primary API areas |
| MCP adapter wiring | `cmd/ptolemy-mcp/main.go` | Connects tool groups to worker API |
| SQLite runtime schema | `internal/store/store.go`, `internal/store/migrations.go` | Defines persistent state tables |
| KB-first memory flow | `internal/navigator/kb.go`, `internal/memory/loader.go` | Shows knowledge base generation and loading |

Related notes:

- [[Runtime Flow]]
- [[Database Schema]]
- [[Agent Loop]]
- [[../02-Implementation-Aspects/Core Runtime]]

