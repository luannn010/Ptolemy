---
priority: normal
task_id: 06-add-client-command-executor
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/06-add-client-command-executor
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on:
  - 05-add-client-file-operations
allowed_files:
  - internal/client/exec/
  - cmd/ptolemy-client/
validation:
  - go test ./internal/client/...
scripts:
  - task-scripts/06-client-command-executor.md
snippets:
  - snippets/client-yaml.example
created_by: chatgpt
---

# Task: Add client command executor

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Keep command execution rooted in the configured workspace.
- Add a policy hook even if it is a simple pass-through for now.

## Inputs
Use these pack files:

- `task-scripts/06-client-command-executor.md`
- `snippets/client-yaml.example`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Implement local command execution using the configured shell and workspace.
3. Capture stdout, stderr, exit code, and timeout behavior.
4. Add a policy hook for future allow/deny checks.
5. Add focused tests and run the listed validation commands.

## Acceptance Checks

- `go test ./internal/client/...`
- Timeout and exit-code behavior are covered by tests

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if command safety requires broader policy design than a hook point.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
