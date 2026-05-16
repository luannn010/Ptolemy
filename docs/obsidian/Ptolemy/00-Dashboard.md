---
project: Ptolemy
category: dashboard
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - obsidian
  - dashboard
---

# Ptolemy Dashboard

## Project summary

Ptolemy is a local-first worker platform for agent-driven coding. The current codebase already includes a usable HTTP worker daemon, SQLite-backed execution state, an MCP adapter, file and Git tooling, a KB-first navigator, a controller-owned agent loop, and a task-pack runtime with an embedded Pack Studio UI.

## Architecture snapshot

- Runtime entrypoints: `cmd/workerd`, `cmd/ptolemy-mcp`, `cmd/ptolemy-agent`, `cmd/ptolemy-task-runner`
- Main state layers: `state/ptolemy.db` plus `.ptolemy/kb/*`
- Primary runtime packages: `internal/httpapi`, `internal/executor`, `internal/terminal`, `internal/agentloop`, `internal/tasks`, `internal/packstudio`
- Primary external interfaces: HTTP API, MCP stdio adapter, local CLI binaries

## Ready-to-use features

- [[02-Implementation-Aspects/Core Runtime]]: `workerd`, sessions, `/execute`, file ops, Git ops, worktree ops, SQLite persistence
- [[02-Implementation-Aspects/Tools]]: controller tools and MCP tool families are implemented in code
- [[02-Implementation-Aspects/Connectors]]: MCP adapter and worker/brain clients are implemented

## Partial features

- [[02-Implementation-Aspects/Security]]: policy, approval records, and task scoping exist, but command policy is still pattern-based
- [[02-Implementation-Aspects/UI]]: Pack Studio UI and run monitoring exist, but this surface is still early-stage
- [[02-Implementation-Aspects/Knowledge Base and Memory]]: KB build/read/update exists, but full refresh automation and broader validation remain limited
- [[02-Implementation-Aspects/Task Packs and Workflow Engine]]: packs, programs, scheduling, and large-task process state exist, but execution is still sequential-first
- [[02-Implementation-Aspects/Observability and Audit]]: actions, logs, approvals, run events, artifacts, and terminal streaming exist, but operations hardening is incomplete
- [[02-Implementation-Aspects/Model and Inference Profiles]]: local brain config and reasoning-profile policy exist, but model-profile breadth is narrow
- [[02-Implementation-Aspects/Testing and Quality]]: many tests exist, but I could not run them in this environment because `go` is unavailable
- [[02-Implementation-Aspects/Deployment and DevOps]]: config and a systemd unit exist, but CI/container/deployment depth is limited
- [[02-Implementation-Aspects/Documentation and Obsidian System]]: this Obsidian layer now exists, but it is still a first audited snapshot

## Blocked or risky features

- [[02-Implementation-Aspects/ChatCLI]] remains a prototype around `cmd/ptolemy-agent`
- [[02-Implementation-Aspects/Distributed Workers]] could not be verified from code and is effectively absent as an implemented subsystem
- Repo-wide `go test ./...` validation is currently blocked here by missing Go tooling, captured in [[07-Test-Reports/Current Test Status]]

## Top 10 next improvements

1. Make `go test ./...` runnable in the standard development environment and CI.
2. Harden command policy beyond substring matching.
3. Add explicit approval/decision APIs rather than only store-level records.
4. Promote KB update flow from partial automation to a tested end-to-end path.
5. Clarify which agent loop path is canonical: `workerd` controller loop vs `ptolemy-agent` prototype.
6. Add stronger Pack Studio and program-run integration tests.
7. Document and test finalization flows for commit, push, PR, and KB update.
8. Replace remaining compatibility-era navigator terminology where KB-first is now canonical.
9. Add deployment automation beyond the single `deploy/workerd.service` unit file.
10. Decide whether a true skills subsystem will exist under `internal/skills/` or remain a design goal only.

## Implementation notes

- [[01-Architecture/System Overview]]
- [[01-Architecture/Runtime Flow]]
- [[01-Architecture/Database Schema]]
- [[01-Architecture/Agent Loop]]

## Aspect notes

- [[02-Implementation-Aspects/Core Runtime]]
- [[02-Implementation-Aspects/Skills]]
- [[02-Implementation-Aspects/Tools]]
- [[02-Implementation-Aspects/Connectors]]
- [[02-Implementation-Aspects/Security]]
- [[02-Implementation-Aspects/ChatCLI]]
- [[02-Implementation-Aspects/UI]]
- [[02-Implementation-Aspects/Knowledge Base and Memory]]
- [[02-Implementation-Aspects/Task Packs and Workflow Engine]]
- [[02-Implementation-Aspects/Observability and Audit]]
- [[02-Implementation-Aspects/Distributed Workers]]
- [[02-Implementation-Aspects/Model and Inference Profiles]]
- [[02-Implementation-Aspects/Testing and Quality]]
- [[02-Implementation-Aspects/Deployment and DevOps]]
- [[02-Implementation-Aspects/Documentation and Obsidian System]]

## Feature and code maps

- [[03-Feature-Matrix/Feature Inventory]]
- [[03-Feature-Matrix/Ready To Use]]
- [[03-Feature-Matrix/Gaps and Roadmap]]
- [[05-Code-Map/Entrypoints]]
- [[05-Code-Map/Package Map]]
- [[05-Code-Map/API Endpoints]]
- [[05-Code-Map/Database Tables]]

## Current useful commands

```bash
go run ./cmd/workerd
go run ./cmd/ptolemy-mcp
go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/templates/task-pack-template
go run ./cmd/ptolemy-task-runner run --pack docs/tasks/templates/task-pack-template --workspace .
go run ./cmd/ptolemy-agent --task-file docs/tasks/inbox/<task>.md --max-steps 8
curl -s http://localhost:8080/health
```

## Current known risks

- The test suite could not be executed from this thread because `go` was not available in either PowerShell or WSL bash.
- The command approval model is narrow and mostly string-pattern based.
- There are multiple active implementation paths for agents, which increases architectural ambiguity.
- Pack and program orchestration is implemented, but reliability under real long-running workloads is not verified here.

## Recommended next documentation updates

- Add per-endpoint example payloads after runtime validation is available.
- Add one note that tracks canonical vs legacy surfaces, especially navigator vs KB terminology.
- Add screenshots or UI state notes for Pack Studio once the app is exercised live.

