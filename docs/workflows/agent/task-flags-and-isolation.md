# Task Flags and Isolation Workflow

Use this workflow when selecting, splitting, or executing task files that carry priority metadata.

## Purpose

Task flags make each task file self-describing so Ptolemy and Codex can safely pick work without mixing tasks, branches, or file scopes across sessions.

## Filename format

New task files must use:

```text
<Priority>-<task-slug>.md
```

Allowed priority prefixes:

- `Urgent`
- `Normal`
- `Low`

Examples:

- `Urgent-fix-worker-eof.md`
- `Normal-split-workflows-by-use-case.md`
- `Low-update-readme.md`

## Required metadata

Every task file must begin with YAML frontmatter.

```yaml
---
priority: urgent
task_id: fix-worker-eof
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/urgent-fix-worker-eof
allowed_files:
  - WORKFLOWS.md
  - AGENTS.md
created_by: codex
---
```

Required fields:

- `priority`: `urgent`, `normal`, or `low`
- `task_id`: a unique identifier for the task
- `parent_task`: `null` for root tasks, or the parent task ID for split tasks
- `owner`: `unassigned` until a session claims the task
- `status`: task lifecycle state such as `inbox`, `active`, `split`, `process`, `done`, or `failed`
- `branch`: the task branch name, using the priority plus task ID pattern
- `allowed_files`: the files or directories this task may modify
- `created_by`: usually `codex`

## Isolation rules

- A Codex/Ptolemy session may only work on one `task_id` at a time.
- It must not edit files outside `allowed_files` unless the task metadata is updated first.
- If a task is already owned or locked by another session, skip it and select another task.
- Use the metadata `branch` value when creating the task branch.
- Keep task edits narrow and local to the declared scope.

## Split task rules

- Split child tasks inherit the parent `priority`.
- Split child tasks set `parent_task` to the parent `task_id`.
- Split child tasks must receive unique `task_id` values.
- Split child tasks may inherit or narrow `allowed_files`, but never broaden them without updating metadata.
- Split child branches must use the child task ID.
- Mark split children with `status: split` until they are selected for execution.

Example child task:

```yaml
---
priority: urgent
task_id: fix-worker-eof-part-1
parent_task: fix-worker-eof
owner: unassigned
status: split
branch: ptolemy/urgent-fix-worker-eof-part-1
allowed_files:
  - WORKFLOWS.md
  - docs/workflows/recovery/eof-worker-drop.md
created_by: codex
---
```

## Priority order

Select work in this order when multiple tasks are available:

1. `urgent`
2. `normal`
3. `low`

Within the same priority, prefer the oldest eligible task that is unlocked and still inside scope.

## Validation and safety

- If frontmatter is missing, malformed, or incomplete, do not guess values.
- Treat the task as invalid until metadata is repaired.
- Do not execute or commit a task without a valid `task_id`, `priority`, `branch`, and `allowed_files` set.
- If the task cannot be validated, stop and route it to a repair or failure path instead of widening the scope.
- Never edit outside `allowed_files` just to make the task easier.

## Reference template

Use `docs/tasks/templates/task-file-template.md` for new root tasks and `docs/tasks/templates/split-task-template.md` for split children.
