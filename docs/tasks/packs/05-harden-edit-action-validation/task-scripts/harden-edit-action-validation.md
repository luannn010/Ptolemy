# Task Script: harden-edit-action-validation

## Intent
Add pre-execution validation for edit/tool actions so incomplete actions become corrective prompts instead of failed execution steps.

## Allowed targets
Search for the current action parsing/execution path and keep edits within the task `allowed_files` only.

Expected likely targets:

- `internal/action/validator.go`
- `internal/action/types.go`
- `internal/action/executor.go`
- `internal/agent/runner.go`
- `internal/agent/*_test.go`
- `internal/action/*_test.go`

## Stable anchors
Use whichever anchors exist in the repository:

- `replace_block`
- `insert_after`
- `create_file`
- `update_file`
- `Action`
- `Execute`
- `Validate`
- `Corrective`
- `Step`

## Referenced snippets

- `snippets/edit-action-validation-rules.md`

## Expected outputs

- Incomplete edit actions are detected before execution.
- Missing fields return a corrective prompt instead of executing the action.
- No repository files are modified when an action is incomplete.
- Existing valid edit actions continue to execute as before.
- Tests cover at least one incomplete action and one valid action path, where practical.

## Must not do

- Do not edit files outside the task's `allowed_files`.
- Do not rewrite whole source files when a targeted edit is possible.
- Do not change unrelated action behavior.
- Do not make the guard dependent on a specific model provider.
- Do not count incomplete action repair as a normal failed execution step if the codebase has a way to distinguish corrective prompts.

## Instructions

1. Locate the code path where model actions are converted into executable file edits.
2. Add a validation layer before action execution.
3. Implement required-field checks for:
   - `replace_block`
   - `insert_after`
   - `create_file`
   - `update_file`
4. Return the corrective prompt text from `snippets/edit-action-validation-rules.md` when fields are missing.
5. Ensure the incomplete action exits before filesystem mutation.
6. Add or update focused tests around validation behavior.
7. Run validation commands from the inbox task.
