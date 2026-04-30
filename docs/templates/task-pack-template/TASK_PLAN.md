# Task Plan: Example Pack

## Goal
Describe the feature this task pack implements.

## Execution Strategy
Use sequential-first execution.

Do not run final integration tasks until all required tasks are completed.

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

## Failure Rule

If any task fails validation, stop immediately and mark the task as failed.
