# Task Plan: Example Pack

## Goal
Describe the single pack-level outcome this pack must deliver.

## Execution Strategy
Use sequential-first execution.

Do not run final integration tasks until all prerequisite tasks are completed.

## Global Constraints

- Do not edit files outside each task's `allowed_files`.
- Do not run scripts from `scripts/` automatically.
- Stop immediately on validation failure unless a task explicitly says otherwise.

## Execution Order

### Phase 1 - Foundation
1. `01-add-validator-model.md`
2. `02-add-task-runner.md`

### Phase 2 - Integration
3. `03-add-cli-command.md`

### Phase 3 - Final Validation
4. `99-final-integration.md`

## Global Validation

```bash
go test ./...
```

## Completion Policy

The pack is complete only when:

- every required task is completed
- task validations pass
- global validation passes
- final integration checks pass

## Failure Rule

If any task fails validation or the agent blocks on an unsafe change, stop immediately and mark the task as failed.
