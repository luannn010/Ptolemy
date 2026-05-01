---
priority: high
task_id: 04-test-mcp-wrapper
parent_task: ptolemy-mcp-wrapper-pack
owner: unassigned
status: inbox
branch: ptolemy/04-test-mcp-wrapper
execution_group: sequential
max_steps: 6
requires_approval: false
stop_on_error: true
depends_on:
  - 03-add-config-and-docs
allowed_files:
  - docs/tasks/packs/ptolemy-mcp-wrapper-pack/
validation:
  - python scripts/mcp/ptolemy_mcp.py --self-test
  - curl -s http://127.0.0.1:8080/health
  - /usr/local/go/bin/go test ./...
scripts:
  - task-scripts/04-smoke-tests.md
snippets:
  - snippets/codex-custom-mcp-config.md
created_by: chatgpt
---

# Task: Test MCP wrapper and record results

## Goal

Run the required smoke tests for the wrapper and record any automation fallback needed for the current pack runner behavior.

## Scope

Only modify files listed in `allowed_files`.

## Constraints

- Prefer reproducible smoke tests over manual observations.
- Record failures with the exact blocking reason.
- Do not run the full pack executor if it would push or create a PR without approval.

## Inputs

Use these pack files:

- `task-scripts/04-smoke-tests.md`
- `snippets/codex-custom-mcp-config.md`

## Execution Steps

1. Run the wrapper self-test.
2. Run a direct worker health check with `curl`.
3. Run the standard repo test command if available in the local environment.
4. Run `go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/packs/ptolemy-mcp-wrapper-pack`.
5. If full automated pack execution is unsafe or fails, write a fallback note under the pack.

## Acceptance Checks

- `python scripts/mcp/ptolemy_mcp.py --self-test`
- `curl -s http://127.0.0.1:8080/health`
- `/usr/local/go/bin/go test ./...`
- Pack planning completes or records a clear failure note

## Failure / Escalation

- Stop if the worker is unreachable and record that exact failure.
- Stop if the standard repo tests fail outside this task scope and record the failure.

## Done When

- [ ] Smoke tests are recorded
- [ ] Validation passes or failures are documented
- [ ] Only allowed files changed
- [ ] Fallback note exists if automation could not safely run
- [ ] Task can be moved from `inbox/` to `done/`
