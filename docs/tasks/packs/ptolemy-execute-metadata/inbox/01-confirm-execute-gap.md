---
priority: normal
task_id: confirm-execute-gap
parent_task: null
owner: unassigned
status: inbox
branch: feature/080526-ptolemy-execute-metadata
allowed_files:
  - docs/tasks/packs/ptolemy-execute-metadata
  - internal/mcp/executortools
  - internal/executor
  - internal/httpapi
validation:
  - rg -n 'ptolemy.execute|/execute' internal/mcp/executortools/tools.go internal/executor/executor.go internal/httpapi/execute.go
created_by: codex
---

# Goal

Confirm that `ptolemy.execute` accepts only the legacy executor fields, routes through `/execute`, and does not yet persist descriptive action metadata on that path.

# Validation

```text
The task output names the exact files to change and confirms backward compatibility constraints.
```
