---
priority: normal
task_id: add-agent-kb-preload-before-scan
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/add-agent-kb-preload-before-scan
allowed_files:
  - cmd/ptolemy-agent
  - internal/navigator
created_by: codex
---

# Goal

Load KB content before calling workspace inspection and place KB content first in the brain prompt.

# Validation

```text
Agent tests prove KB prompt content is available before snapshot text.
```
