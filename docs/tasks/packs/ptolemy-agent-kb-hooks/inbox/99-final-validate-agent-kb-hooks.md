---
priority: normal
task_id: final-validate-agent-kb-hooks
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/final-validate-agent-kb-hooks
allowed_files:
  - cmd/ptolemy-agent
  - internal/tasks
  - .ptolemy
created_by: codex
---

# Goal

Run final validation for the agent preload and pack-success KB update flow.

# Validation

```bash
/usr/local/go/bin/go test ./cmd/ptolemy-agent ./internal/tasks
```
