---
priority: normal
task_id: 03-add-config-and-docs
parent_task: ptolemy-mcp-wrapper-pack
owner: unassigned
status: inbox
branch: ptolemy/03-add-config-and-docs
execution_group: sequential
max_steps: 6
requires_approval: false
stop_on_error: true
depends_on:
  - 02-add-mcp-wrapper
allowed_files:
  - README.md
  - docs/Worker_API.md
  - docs/tasks/packs/ptolemy-mcp-wrapper-pack/
validation:
  - python scripts/mcp/ptolemy_mcp.py --self-test
scripts:
  - task-scripts/03-config-and-docs.md
snippets:
  - snippets/codex-custom-mcp-config.md
created_by: chatgpt
---

# Task: Add MCP wrapper configuration and docs

## Goal

Document how to run the new wrapper in Codex Custom MCP and how it maps to the existing Ptolemy worker.

## Scope

Only modify files listed in `allowed_files`.

## Constraints

- Show the exact Codex Custom MCP fields.
- Do not include real secrets or environment values.
- Keep the docs narrow and wrapper-focused.

## Inputs

Use these pack files:

- `task-scripts/03-config-and-docs.md`
- `snippets/codex-custom-mcp-config.md`

## Execution Steps

1. Update repo docs with the wrapper purpose and environment variables.
2. Add the exact Codex Custom MCP UI values:
   - Name `Ptolemy`
   - Type `STDIO`
   - Command `python`
   - Arguments `scripts/mcp/ptolemy_mcp.py`
   - Working directory repo root
   - Environment variable `PTOLEMY_BASE_URL=http://<tailscale-ip>:8080`
3. Document optional `PTOLEMY_DEFAULT_SESSION_ID` and `PTOLEMY_AUTH_TOKEN`.
4. Mention the `/agent/run` fallback behavior explicitly.

## Acceptance Checks

- `python scripts/mcp/ptolemy_mcp.py --self-test`
- Docs include the exact Codex MCP UI configuration.
- Docs do not expose secrets.

## Failure / Escalation

- Stop if the required docs change would sprawl beyond the listed files.
- Stop if the wrapper behavior no longer matches the documented worker API.

## Done When

- [ ] Docs are updated
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
