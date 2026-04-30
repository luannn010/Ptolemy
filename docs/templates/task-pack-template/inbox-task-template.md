---
priority: normal
task_id: <unique-task-id>
parent_task: <pack-id>
owner: unassigned
status: inbox
branch: ptolemy/<task-id>
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on: []
allowed_files:
  - <file-path>
validation:
  - go test ./internal/tasks
scripts:
  - task-scripts/<script-name>.md
snippets:
  - snippets/<snippet-file>
created_by: chatgpt
---

# Task: <Short title>

## Goal
One clear outcome.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Stop and explain if the task would require broader edits.

## Inputs
Use these pack files:

- `task-scripts/<script-name>.md`
- `snippets/<snippet-file>`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Apply only the smallest change needed to satisfy the goal.
3. Keep edits within `allowed_files`.
4. Run the listed validation commands after editing.

## Acceptance Checks

- `go test ./internal/tasks`
- Any task-specific checks needed for this change

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if a required referenced asset is missing or ambiguous.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
