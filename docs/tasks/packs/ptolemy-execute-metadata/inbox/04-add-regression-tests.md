---
priority: normal
task_id: add-execute-metadata-regression-tests
parent_task: null
owner: unassigned
status: inbox
branch: feature/080526-ptolemy-execute-metadata
depends_on:
  - persist-execute-action-metadata
allowed_files:
  - internal/mcp/executortools
  - internal/executor
  - internal/httpapi
validation:
  - /usr/local/go/bin/go test ./internal/mcp/executortools ./internal/executor ./internal/httpapi
created_by: codex
---

# Goal

Add focused regression tests for schema exposure, `/execute` acceptance of the new fields, backward compatibility, and descriptive metadata persistence.

# Validation

```bash
/usr/local/go/bin/go test ./internal/mcp/executortools ./internal/executor ./internal/httpapi
```
