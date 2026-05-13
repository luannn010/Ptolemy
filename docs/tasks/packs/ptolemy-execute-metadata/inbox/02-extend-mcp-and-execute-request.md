---
priority: normal
task_id: extend-mcp-and-execute-request
parent_task: null
owner: unassigned
status: inbox
branch: feature/080526-ptolemy-execute-metadata
depends_on:
  - confirm-execute-gap
allowed_files:
  - internal/mcp/executortools
  - internal/executor
validation:
  - rg -n 'title|purpose|reasoning_step|target' internal/mcp/executortools/tools.go internal/executor/executor.go
created_by: codex
---

# Goal

Add optional `title`, `purpose`, `reasoning_step`, and `target` fields to the `ptolemy.execute` MCP schema and the `/execute` request model without changing existing legacy fields.

# Validation

```text
Tool schema and ExecuteRequest both expose the new optional fields while old requests remain valid.
```
