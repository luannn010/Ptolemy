---
project: Ptolemy
category: architecture
status: Ready
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - database
---

# Database Schema

Ptolemy uses SQLite for runtime and audit state. The schema is split between a base store migration for `sessions` and `command_logs`, and a later migration pass that adds actions, approvals, logs, agent runs, observations, program runs, pack runs, pack tasks, run events, and supporting indexes.

Schema evidence:

| Evidence | Path | Notes |
|---|---|---|
| Base SQLite store and first tables | `internal/store/store.go` | Creates `sessions` and `command_logs` |
| Extended runtime migrations | `internal/store/migrations.go` | Creates operational and Pack Studio tables |
| Session persistence use | `internal/session/store.go` | Confirms active use of `sessions` |
| Agent-run persistence use | `internal/agentloop/store.go` | Confirms active use of `agent_runs` and `agent_observations` |

See also:

- [[../05-Code-Map/Database Tables]]
- [[../02-Implementation-Aspects/Observability and Audit]]

