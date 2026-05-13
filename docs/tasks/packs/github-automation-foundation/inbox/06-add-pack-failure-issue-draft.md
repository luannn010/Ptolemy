---
priority: normal
task_id: add-pack-failure-issue-draft
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-failure-issue-draft
execution_group: sequential
depends_on:
  - add-pack-artifact-logging
  - add-pack-branch-preparation
allowed_files:
  - internal/tasks/pack_runtime.go
  - internal/tasks/pack_test.go
validation:
  - go test ./internal/tasks
created_by: codex
---

# Task: Add pack failure issue draft

## Goal

When a task pack fails, create a local issue draft artifact that captures what a human should raise on GitHub.

## Required behavior

- write a Markdown issue draft under the pack artifact directory
- include the failed task, branch, and log path
- keep the behavior local-first and deterministic

## Done when

Failure artifacts are good enough for a human or future GitHub automation layer to file an issue quickly.
