---
priority: high
task_id: 05-harden-edit-action-validation
parent_task: 05-harden-edit-action-validation
owner: unassigned
status: inbox
branch: ptolemy/05-harden-edit-action-validation
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on: []
allowed_files:
  - internal/action/validator.go
  - internal/action/types.go
  - internal/action/executor.go
  - internal/action/validator_test.go
  - internal/action/executor_test.go
  - internal/agent/runner.go
  - internal/agent/runner_test.go
  - docs/memory/projects/ptolemy/decisions.md
validation:
  - go test ./internal/action
  - go test ./internal/agent
  - go test ./internal/...
  - go test ./...
scripts:
  - task-scripts/harden-edit-action-validation.md
snippets:
  - snippets/edit-action-validation-rules.md
created_by: chatgpt
---

# Task: Harden edit action validation

## Goal
Validate edit action fields before execution so incomplete actions are rejected safely and returned to the model as corrective prompts instead of failed/burned steps.

## Scope
Only modify files listed in `allowed_files`.

This task should add or update the smallest validation layer needed in the existing action/agent execution path.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Stop and explain if the task would require broader edits.
- Do not execute incomplete edit actions.
- Do not modify repository files when the action is incomplete.
- Keep existing successful action behavior unchanged.

## Inputs
Use these pack files:

- `task-scripts/harden-edit-action-validation.md`
- `snippets/edit-action-validation-rules.md`

## Execution Steps
1. Read the linked task script and referenced snippet before editing.
2. Inspect the existing action execution flow for `replace_block`, `insert_after`, `create_file`, and `update_file`.
3. Add pre-execution validation for required fields:
   - `replace_block`: `target_file`, `new_block`, and either `old_block` or `anchor`
   - `insert_after`: `target_file`, `anchor`, `snippet`
   - `create_file`: `target_file`, `content`
   - `update_file`: `target_file`, and either `content` or `patch`
4. If fields are missing, return a corrective prompt and skip execution.
5. Add focused tests proving incomplete actions do not mutate files and return corrective prompts.
6. Run the listed validation commands after editing.

## Acceptance Checks

- `go test ./internal/action`
- `go test ./internal/agent`
- `go test ./internal/...`
- `go test ./...`
- Incomplete `replace_block` returns a corrective prompt before execution.
- Incomplete `insert_after` returns a corrective prompt before execution.
- Incomplete `create_file` returns a corrective prompt before execution.
- Incomplete `update_file` returns a corrective prompt before execution.
- Valid edit actions still follow the existing execution path.

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if a required referenced asset is missing or ambiguous.
- Stop if validation fails and the issue is not clearly within task scope.
- Stop if the repository uses different file paths for action execution and those files are not listed in `allowed_files`.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Incomplete edit actions do not mutate files
- [ ] Corrective prompts are returned for incomplete edit actions
- [ ] Task can be moved from `inbox/` to `done/`
