---
priority: normal
task_id: final-validate-kb-migration
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/final-validate-kb-migration
allowed_files:
  - internal/httpapi
  - internal/mcp
  - docs/tasks/templates
  - docs/tasks/packs
created_by: codex
---

# Goal

Run final validation for the KB alias surface and docs migration slice.

# Validation

```bash
/usr/local/go/bin/go test ./internal/httpapi ./internal/mcp/...
```
