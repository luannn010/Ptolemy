---
priority: normal
task_id: add-pack-success-pr-draft
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-success-pr-draft
execution_group: sequential
depends_on:
  - add-pack-artifact-logging
  - add-pack-branch-preparation
allowed_files:
  - internal/tasks/pack.go
  - internal/tasks/pack_test.go
validation:
  - go test ./internal/tasks
created_by: codex
---

# Task: Add pack success pull request draft

## Goal

When a task pack succeeds, create a local pull request draft artifact for merging work back to `main`.

## Required behavior

- write a Markdown pull request draft under the pack artifact directory
- include completed tasks and prepared branches
- target `main` as the merge base in the draft

## Done when

Successful pack runs leave behind a clear PR handoff artifact.
