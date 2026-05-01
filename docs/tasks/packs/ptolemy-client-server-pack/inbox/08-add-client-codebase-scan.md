---
priority: normal
task_id: 08-add-client-codebase-scan
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/08-add-client-codebase-scan
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on:
  - 07-add-client-skill-sync
allowed_files:
  - internal/client/scan/
  - cmd/ptolemy-client/
validation:
  - go test ./internal/client/...
scripts:
  - task-scripts/08-client-codebase-scan.md
snippets:
  - snippets/client-init-tree.txt
created_by: chatgpt
---

# Task: Add client codebase scan

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Keep scanning local to the target workspace.
- Stop and explain if richer indexing requires broader repo changes.

## Inputs
Use these pack files:

- `task-scripts/08-client-codebase-scan.md`
- `snippets/client-init-tree.txt`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Implement the client codebase scan behavior for the local workspace.
3. Store output in the client-owned `.ptolemy/` area only.
4. Add focused tests and run the listed validation commands.

## Acceptance Checks

- `go test ./internal/client/...`
- Scan output is deterministic and stays within the client workspace

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if the task needs worker/server-side indexing to be useful.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
