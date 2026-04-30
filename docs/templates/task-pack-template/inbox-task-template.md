---
priority: normal
task_id: <unique-task-id>
parent_task: <pack-id>
owner: unassigned
status: inbox
branch: ptolemy/<task-id>
execution_group: sequential
depends_on: []
allowed_files:
  - <file-path>
validation:
  - go test ./internal/tasks
scripts:
  - task-scripts/<script-name>.md
snippets:
  - snippets/<snippet-file>
created_by: chatgpt
---

# Task: <Short title>

## Goal
One clear outcome.

## Scope
Only modify files listed in `allowed_files`.

## Inputs
Use these pack files:

- `task-scripts/<script-name>.md`
- `snippets/<snippet-file>`

## Required behavior
Describe exactly what the task must do.

## File operations
1. Create or update `<file-path>`.
2. Use snippet from `snippets/<snippet-file>`.
3. Preserve existing unrelated code.

## Validation

```bash
go test ./internal/tasks
```

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
