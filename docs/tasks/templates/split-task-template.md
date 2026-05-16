---
priority: normal
task_id: example-task-id-part-1
parent_task: example-task-id
owner: unassigned
status: split
branch: ptolemy/normal-example-task-id-part-1
allowed_files:
  - WORKFLOWS.md
  - docs/workflows/agent/task-flags-and-isolation.md
created_by: codex
---

# Parent Task

Reference the parent task file and describe how this child fits into the split.

# Goal

Describe the child task outcome in one short paragraph.

# Scope

List the narrowed files, folders, or workflows this child may modify.

# Steps

1. Review the parent task and inherited scope.
2. Confirm the child metadata is valid.
3. Keep the work inside the child `allowed_files` list.
4. Validate the result before handing off.

# Validation

List the commands that must pass before the child task is complete.

# Commit Message

```text
feat: describe the split task change
```
