---
priority: normal
task_id: 01-add-server-skill-registry
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/01-add-server-skill-registry
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on: []
allowed_files:
  - internal/skills/
  - internal/httpapi/
  - cmd/workerd/
  - docs/skills/
validation:
  - go test ./...
scripts:
  - task-scripts/01-server-skill-registry.md
snippets:
  - snippets/client-server-architecture.md
created_by: chatgpt
---

# Task: Add server skill registry

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not require Docker.
- Preserve unrelated code and existing server behavior.
- Stop if the task would require edits outside `allowed_files`.

## Inputs
Use these pack files:

- `task-scripts/01-server-skill-registry.md`
- `snippets/client-server-architecture.md`

## Execution Steps
1. Read `task-scripts/01-server-skill-registry.md` and `snippets/client-server-architecture.md` before editing.
2. Implement the server-side skill registry in the allowed files only.
3. Add or update tests for the new registry behavior where possible.
4. Run the validation commands after the code changes are in place.

## Acceptance Checks

- `go test ./...`

## Failure / Escalation

- Stop if the required pack assets are missing or contradictory.
- Stop if the change needs files outside `allowed_files`.
- Stop if validation fails and the fix clearly belongs to a different task.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
