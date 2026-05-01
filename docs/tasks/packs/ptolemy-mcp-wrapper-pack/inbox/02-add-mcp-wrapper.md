---
priority: high
task_id: 02-add-mcp-wrapper
parent_task: ptolemy-mcp-wrapper-pack
owner: unassigned
status: inbox
branch: ptolemy/02-add-mcp-wrapper
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on:
  - 01-discover-worker-api
allowed_files:
  - scripts/mcp/ptolemy_mcp.py
  - docs/tasks/packs/ptolemy-mcp-wrapper-pack/
validation:
  - python scripts/mcp/ptolemy_mcp.py --self-test
scripts:
  - task-scripts/02-mcp-wrapper-implementation.md
snippets:
  - snippets/codex-custom-mcp-config.md
created_by: chatgpt
---

# Task: Add Python STDIO MCP wrapper

## Goal

Implement a small MCP bridge that maps STDIO MCP requests to the existing Ptolemy worker HTTP API.

## Scope

Only modify files listed in `allowed_files`.

## Constraints

- Keep the wrapper thin and stateless.
- Use Python stdlib unless a missing capability forces a dependency.
- Do not replace the existing Go worker or expose secrets.
- Return clear errors when the worker is unreachable or an endpoint is unavailable.

## Inputs

Use these pack files:

- `task-scripts/02-mcp-wrapper-implementation.md`
- `snippets/codex-custom-mcp-config.md`

## Execution Steps

1. Implement newline-delimited JSON-RPC handling for `initialize`, `tools/list`, and `tools/call`.
2. Add MCP tools for health, session creation, command execution, and best-effort task-file execution.
3. Use environment variables for base URL, default session ID, and optional bearer token support.
4. Add safe default HTTP timeouts and clean tool responses.
5. Add a `--self-test` mode that checks the configured worker health endpoint.

## Acceptance Checks

- `python scripts/mcp/ptolemy_mcp.py --self-test`
- The wrapper can answer `initialize` and `tools/list` over STDIO.
- The wrapper returns a clean error when `/agent/run` is unavailable.

## Failure / Escalation

- Stop if the wrapper requires dependencies outside standard Python.
- Stop if the implementation needs unrelated worker refactors.

## Done When

- [ ] Wrapper script is complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] MCP tools match the required bridge behavior
- [ ] Task can be moved from `inbox/` to `done/`
