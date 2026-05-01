# Task Plan: Harden Edit Action Validation

## Goal
Patch `ptolemy-agent` so incomplete edit actions are rejected before execution and converted into a corrective prompt instead of consuming a failed execution step.

## Execution Strategy
Use sequential-first execution.

This pack contains one focused task because the change should be narrow: validate edit action fields before any filesystem mutation occurs.

## Global Constraints

- Do not edit files outside the task's `allowed_files`.
- Do not run scripts from `scripts/` automatically.
- Do not modify repository files when an edit action is incomplete.
- Stop immediately if the required agent/action execution files are not inside the allowed file list.
- Preserve unrelated code and user changes.

## Execution Order

### Phase 1 - Guard incomplete edit actions
1. `05-harden-edit-action-validation.md`

## Required Behavior

Before execution, validate these actions:

```text
replace_block:
  require target_file
  require new_block
  require old_block or anchor

insert_after:
  require target_file, anchor, snippet

create_file:
  require target_file, content

update_file:
  require target_file
  require content or patch
```

If required fields are missing:

```text
- do not execute the action
- do not modify repo files
- return a corrective prompt asking the model to repair the action
```

Canonical corrective prompt for `replace_block`:

```text
Your replace_block action is incomplete. Return exactly one JSON object with target_file, old_block, and new_block.
```

Equivalent prompts should be returned for `insert_after`, `create_file`, and `update_file`.

## Global Validation

```bash
go test ./internal/...
go test ./...
```

## Completion Policy

The pack is complete only when:

- the guard exists before edit action execution
- incomplete actions do not modify files
- the agent receives a corrective prompt instead of a burned failed step
- task validation passes
- global validation passes

## Failure Rule

If validation fails or the implementation requires editing files outside the task contract, stop immediately and explain the blocker.
