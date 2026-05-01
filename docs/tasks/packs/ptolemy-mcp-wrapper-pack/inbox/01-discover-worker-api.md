---
priority: high
task_id: 01-discover-worker-api
parent_task: ptolemy-mcp-wrapper-pack
owner: unassigned
status: inbox
branch: ptolemy/01-discover-worker-api
execution_group: sequential
max_steps: 6
requires_approval: false
stop_on_error: true
depends_on: []
allowed_files:
  - docs/tasks/packs/ptolemy-mcp-wrapper-pack/
  - docs/Worker_API.md
validation:
  - curl -s http://127.0.0.1:8080/health
scripts:
  - task-scripts/01-worker-api-discovery.md
snippets:
  - snippets/codex-custom-mcp-config.md
created_by: chatgpt
---

# Task: Discover worker API shape for MCP wrapper

## Goal

Confirm the current worker endpoints, payloads, and documented gaps before the bridge implementation is finalized.

## Scope

Only modify files listed in `allowed_files`.

## Constraints

- Do not guess undocumented endpoints when repo evidence disagrees.
- Keep the discovery output focused on wrapper-relevant endpoints.
- Stop if the repo no longer exposes the documented health, sessions, or execute endpoints.

## Inputs

Use these pack files:

- `task-scripts/01-worker-api-discovery.md`
- `snippets/codex-custom-mcp-config.md`

## Execution Steps

1. Read the worker API docs and the router or handler code for `/health`, `/sessions`, `/execute`, and task endpoints.
2. Record the confirmed endpoint shapes and note that `/agent/run` is optional and may be unavailable.
3. Update pack-local docs if the implementation plan needs a clarified fallback.
4. Run the validation command after the discovery notes are in place.

## Acceptance Checks

- `curl -s http://127.0.0.1:8080/health`
- Pack docs describe the confirmed worker bridge contract.
- Pack docs mention the fallback when `/agent/run` is absent.

## Failure / Escalation

- Stop if the documented worker endpoints cannot be reconciled with the current router.
- Stop if the discovery requires unrelated code edits outside `allowed_files`.

## Done When

- [ ] Discovery notes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
