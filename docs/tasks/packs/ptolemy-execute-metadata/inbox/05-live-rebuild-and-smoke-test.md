---
priority: normal
task_id: live-rebuild-and-smoke-test-execute-metadata
parent_task: null
owner: unassigned
status: inbox
branch: feature/080526-ptolemy-execute-metadata
depends_on:
  - add-execute-metadata-regression-tests
allowed_files:
  - docs/tasks/packs/ptolemy-execute-metadata
validation:
  - /usr/local/go/bin/go build ./cmd/workerd
created_by: codex
---

# Goal

Rebuild `workerd`, restart the live service if needed, run a metadata-rich Ptolemy execution through the live executor path, and confirm the newest `command.exec` action row in `state/ptolemy.db` contains `title`, `purpose`, `reasoning_step`, and `target`.

# Validation

```bash
/usr/local/go/bin/go build ./cmd/workerd
```
