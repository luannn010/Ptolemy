---
priority: normal
task_id: 99-final-integration
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/99-final-integration
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on:
  - 09-add-client-server-workflow-docs
allowed_files:
  - cmd/
  - internal/
  - docs/
  - AGENTS.md
  - WORKFLOWS.md
validation:
  - go test ./...
  - go build ./cmd/workerd
  - go build ./cmd/ptolemy-client
scripts:
  - task-scripts/99-final-integration.md
snippets:
  - snippets/client-init-tree.txt
  - snippets/client-yaml.example
created_by: chatgpt
---

# Task: Final integration validation

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Prefer validation and small integration fixes over broad redesign.
- Stop and explain if final validation uncovers a cross-cutting issue too large for this task.

## Inputs
Use these pack files:

- `task-scripts/99-final-integration.md`
- `snippets/client-init-tree.txt`
- `snippets/client-yaml.example`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Run final integration checks across the implemented client-server pieces.
3. Make only the smallest integration fixes needed to satisfy validation.
4. Run all listed validation commands after editing.

## Acceptance Checks

- `go test ./...`
- `go build ./cmd/workerd`
- `go build ./cmd/ptolemy-client`

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if final validation reveals a major missing phase rather than a narrow integration bug.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
