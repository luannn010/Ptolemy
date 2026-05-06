---
priority: normal
task_id: switch-memory-loader-to-kb-first
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/switch-memory-loader-to-kb-first
allowed_files:
  - internal/memory
  - internal/navigator
created_by: codex
---

# Goal

Make the workspace memory loader prefer `.ptolemy/kb` before legacy `.ptolemy/context` and `docs/memory`.

# Validation

```text
Workspace memory loader tests cover KB-first, context fallback, and docs fallback behavior.
```
