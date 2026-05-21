---
project: Ptolemy
category: architecture
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - runtime
---

# Runtime Flow

The current runtime flow starts with `workerd` loading configuration, opening SQLite, running migrations, creating stores, bootstrapping the tmux-backed runner, registering agent-loop tools, recovering Pack Studio state, and mounting the HTTP router. Most user-facing work then enters through `/sessions`, `/execute`, `/agent-runs`, `/file/*`, `/git/*`, `/worktree/*`, `/kb/*`, or `/ui/*`.

For command execution, `executor.Executor` validates an open session, creates an action record, runs the command through `terminal.TmuxRunner`, truncates output if needed, writes logs and command history, and returns a JSON response. For agent execution, `agentloop.Service` loads task scope, ensures a session, asks the brain for one JSON action, routes that action through the worker-owned tool registry, records observations, and optionally finalizes.

Key evidence:

| Evidence | Path | Notes |
|---|---|---|
| Main bootstrap path | `cmd/workerd/main.go` | Startup, stores, runner, router |
| Command execution path | `internal/executor/executor.go` | Action, log, command log, output truncation |
| tmux execution path | `internal/terminal/tmux_runner.go` | Session-backed command execution |
| Agent run lifecycle | `internal/agentloop/service.go` | Start/resume and task loading |
| Task-pack run startup | `internal/packstudio/service.go`, `internal/tasks/pack.go` | Program and pack orchestration |

Related notes:

- [[System Overview]]
- [[Agent Loop]]
- [[../04-Workflows/Codex to Ptolemy Flow]]

