---
project: Ptolemy
category: security
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Security

## What this aspect means

Security and governance cover command policy, approval gates, task file scoping, path restrictions, audit trails, and rules that stop agents from acting outside declared bounds.

## Current implementation status

Status: `Partial`

Ptolemy has a real command policy, approval records, task metadata validation, and allowed-files concepts. The policy implementation is still narrow and largely based on string matching rather than deep command analysis.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Command policy | `internal/policy/policy.go` | Allow/ask/deny pattern rules |
| Approval persistence | `internal/approval/store.go` | Approval records stored in SQLite |
| Command handler policy checks | `internal/httpapi/commands.go` | Enforces deny/ask flow |
| Task validation and metadata model | `internal/tasks/validator.go`, `internal/tasks/task.go`, `docs/workflows/agent/task-flags-and-isolation.md` | Scope and branch governance |
| Agent-loop path controls | `internal/agentloop/tool_executor.go` | Uses task scope for actions |

## Ready-to-use capabilities

- Approval-required handling for selected risky commands
- Deny rules for some obvious secret reads
- Task metadata structure for branch and file isolation

## Partial or unfinished capabilities

- Policy rules are substring-based
- No rich RBAC or multi-user governance model was found
- Approval decision APIs were not found in the HTTP surface

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| Policy check | `internal/policy.CheckCommand` | Command decision | Partial |
| Approval store | `internal/approval/store.go` | Persist approvals | Partial |
| Task isolation workflow | `docs/workflows/agent/task-flags-and-isolation.md` | Governance guidance | Partial |

## Important commands

```bash
curl -X POST http://localhost:8080/sessions/<id>/commands
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/policy/policy_test.go` | Policy decisions | Test file present |
| `internal/tasks/validator_test.go` | Task metadata validation | Test file present |
| `go test ./...` | End-to-end validation | Could not run here because `go` was unavailable |

## Risks

- Simple string matching can under-classify risky commands.
- There is no evidence of sandboxing inside Ptolemy itself beyond policy and scope checks.

## Gaps

- Missing structured approval workflow endpoints.
- Missing policy explainability docs for operators.

## Next recommended improvements

- Move policy from string rules to parsed command semantics.
- Expose approval resolution APIs and audit views.

