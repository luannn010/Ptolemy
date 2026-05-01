---
priority: normal
task_id: 05-add-client-file-operations
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/05-add-client-file-operations
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on:
  - 04-add-client-workspace-guard
allowed_files:
  - internal/client/fileops/
  - cmd/ptolemy-client/
validation:
  - go test ./internal/client/...
scripts:
  - task-scripts/05-client-file-operations.md
snippets:
  - snippets/client-server-architecture.md
created_by: chatgpt
---

# Task: Add client file operations

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Use the workspace guard for all path-sensitive operations.
- Stop and explain if the task would require broader edits.

## Inputs
Use these pack files:

- `task-scripts/05-client-file-operations.md`
- `snippets/client-server-architecture.md`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Implement client-side file operations rooted in the guarded workspace.
3. Expose only the smallest CLI surface needed for this task.
4. Add tests for success and guard-failure cases.
5. Run the listed validation commands after editing.

## Acceptance Checks

- `go test ./internal/client/...`
- File operations stay within the workspace and fail safely otherwise

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if command or network behaviors are needed to satisfy this task.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
