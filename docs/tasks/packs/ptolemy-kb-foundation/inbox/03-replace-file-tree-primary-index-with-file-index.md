---
priority: normal
task_id: replace-file-tree-primary-index-with-file-index
parent_task: null
owner: unassigned
status: inbox
branch: ptolemy/replace-file-tree-primary-index-with-file-index
allowed_files:
  - internal/navigator
  - .ptolemy
created_by: codex
---

# Goal

Make `FILE_INDEX.json` the primary machine-readable file map while continuing to emit `file-tree.json` as a compatibility artifact.

# Validation

```text
FILE_INDEX.json is written and file-tree.json still builds successfully.
```
