---
priority: normal
task_id: 09-add-client-server-workflow-docs
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/09-add-client-server-workflow-docs
execution_group: sequential
max_steps: 6
requires_approval: false
stop_on_error: true
depends_on:
  - 08-add-client-codebase-scan
allowed_files:
  - docs/workflows/
  - AGENTS.md
  - WORKFLOWS.md
validation:
  - go test ./...
scripts:
  - task-scripts/09-client-server-docs.md
snippets:
  - snippets/client-server-architecture.md
created_by: chatgpt
---

# Task: Add client server workflow docs

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Keep docs aligned with the implemented client-server behavior.
- Stop and explain if undocumented runtime gaps block accurate docs.

## Inputs
Use these pack files:

- `task-scripts/09-client-server-docs.md`
- `snippets/client-server-architecture.md`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Update workflow and operator docs for the client-server model.
3. Keep instructions consistent with current commands and endpoints.
4. Run the listed validation commands after editing.

## Acceptance Checks

- `go test ./...`
- Docs accurately describe the current client-server workflow and constraints

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if the docs would need to promise behavior not implemented yet.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
