---
priority: high
task_id: add-pack-integration-branch-merge
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-integration-branch-merge
execution_group: sequential
depends_on:
  - add-pack-artifact-logging
  - add-pack-branch-preparation
allowed_files:
  - internal/tasks/pack_runtime.go
  - internal/tasks/pack_test.go
  - internal/worktree/worktree.go
validation:
  - go test ./internal/tasks
created_by: codex
---

# Task: Add pack integration branch merge

## Goal

After all task branches are ready, merge them into one final integration branch in dependency order.

## Required behavior

- create or reuse a final integration branch for the pack
- merge prepared task branches into that branch in planned order
- keep the main workspace branch unchanged by doing the merge in an isolated worktree
- save merge logs as artifacts

## Done when

Successful pack runs converge from multiple task branches into one final branch ready for publish.
