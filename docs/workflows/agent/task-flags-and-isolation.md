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
- Keep each child task to one narrow phase such as `inspect`, `plan`, `edit`, `test`, `validate`, `docs`, or `finalize`.
- Keep each child task body within a model-friendly size when possible:
  under roughly 1200-2000 chars is comfortable and above 4000 chars is risky.
- Child tasks should use compact manifest/state files as memory rather than relying on full prior chat history.

## Process manifests

When a large task is executed as a process pack, Ptolemy writes:

- `.ptolemy/tasks/process/<pack-id>/manifest.json`
- `.ptolemy/tasks/process/<pack-id>/todo.md`
- `.ptolemy/tasks/process/<pack-id>/state/<child-id>-result.md`

The manifest records:

- `pack_id`
- `parent_task_file`
- `branch`
- `status`
- `current_child_task_id`
- `child_tasks`
- `dependencies`
- `allowed_files`
- `validation_commands`
- per-child `result_summary_path`, `files_changed`, `commands_run`, and `failure_reason`

Each child task starts with a fresh context assembled from:

1. the fixed system prompt
2. the current child task only
3. the relevant manifest slice
4. compact previous child summaries
5. budgeted `.ptolemy` context snippets

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
