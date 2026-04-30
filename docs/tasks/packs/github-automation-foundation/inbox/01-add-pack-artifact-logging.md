---
priority: high
task_id: add-pack-artifact-logging
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-artifact-logging
execution_group: sequential
allowed_files:
  - internal/tasks/pack.go
  - internal/tasks/pack_test.go
validation:
  - go test ./internal/tasks
created_by: codex
---

# Task: Add pack artifact logging

## Goal

Persist task-pack execution artifacts so a pack run can be inspected after completion or failure.

## Required behavior

- write per-task execution logs under `.state/task-packs/<pack-id>/tasks`
- write a pack summary artifact under `.state/task-packs/<pack-id>`
- record artifact paths in the pack run result

## Done when

Task-pack execution leaves behind deterministic local artifacts without changing the loose-task runner behavior.
