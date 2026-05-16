---
project: Ptolemy
category: architecture
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - agent-loop
---

# Agent Loop

Ptolemy now contains a controller-owned agent loop inside `workerd`. The worker loads a task, ensures a session, queries the configured brain, validates single-object JSON actions, executes the allowed tool through the registry, records observations, and continues until completion, cancellation, or guardrail failure.

This controller loop is distinct from the older `cmd/ptolemy-agent` prototype. The prototype still exists and remains useful as an implementation reference, but the more strategic direction in the codebase is the `internal/agentloop` service used by `/agent-runs` and Pack Studio program execution.

Key evidence:

| Evidence | Path | Notes |
|---|---|---|
| Agent run HTTP routes | `internal/httpapi/agent_runs.go` | Create, inspect, resume, cancel, list actions/observations |
| Controller entry service | `internal/agentloop/service.go` | Start/resume, ensure session, load task |
| Tool registry and executors | `internal/agentloop/tool_registry.go`, `internal/agentloop/tool_executor.go` | Worker-owned tool dispatch |
| Reasoning profile policy | `internal/agentloop/reasoning_profile.go` | Workerd controls reasoning profile |
| Legacy local loop prototype | `cmd/ptolemy-agent/main.go` | Still present, but separate path |

Related notes:

- [[../02-Implementation-Aspects/ChatCLI]]
- [[../02-Implementation-Aspects/Model and Inference Profiles]]
- [[../04-Workflows/Failure and Resume Flow]]

