---
priority: normal
task_id: persist-execute-action-metadata
parent_task: null
owner: unassigned
status: inbox
branch: feature/080526-ptolemy-execute-metadata
depends_on:
  - extend-mcp-and-execute-request
allowed_files:
  - internal/executor
  - internal/httpapi
validation:
  - rg -n 'command.exec|MergeMetadata|UpdateResult' internal/executor/executor.go internal/httpapi/router.go
created_by: codex
---

# Goal

Make `/execute` create `command.exec` action and log records that merge the descriptive metadata into `actions.metadata`, with `target` defaulting to the command when omitted.

# Validation

```text
Running the executor path stores command, action, and log records with merged metadata.
```
