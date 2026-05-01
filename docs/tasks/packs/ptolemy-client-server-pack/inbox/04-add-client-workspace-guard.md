---
priority: normal
task_id: 04-add-client-workspace-guard
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/04-add-client-workspace-guard
execution_group: sequential
max_steps: 6
requires_approval: false
stop_on_error: true
depends_on:
  - 03-add-client-config-and-init
allowed_files:
  - internal/client/workspace/
validation:
  - go test ./internal/client/...
scripts:
  - task-scripts/04-client-workspace-guard.md
snippets:
  - snippets/client-server-architecture.md
created_by: chatgpt
---

# Task: Add client workspace guard

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Stop and explain if the task would require broader edits.

## Inputs
Use these pack files:

- `task-scripts/04-client-workspace-guard.md`
- `snippets/client-server-architecture.md`

## Execution Steps
1. Read the exact repo-relative task script and snippet paths listed above before editing.
2. Implement absolute workspace resolution plus guarded path resolution.
3. Reject traversal and practical escape cases, including obvious absolute paths outside the workspace.
4. Add unit tests for valid paths, traversal, and outside-workspace paths.
5. Run the listed validation commands after editing.

## Acceptance Checks

- `go test ./internal/client/...`
- Workspace guard behavior is covered by focused unit tests

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if the task needs deeper symlink-handling behavior than the current scope supports safely.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
