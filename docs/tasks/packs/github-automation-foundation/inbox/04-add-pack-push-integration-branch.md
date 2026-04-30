---
priority: normal
task_id: add-pack-push-integration-branch
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-push-integration-branch
execution_group: sequential
depends_on:
  - add-pack-integration-branch-merge
allowed_files:
  - internal/tasks/pack_runtime.go
  - internal/tasks/pack_test.go
  - internal/gitops/gitops.go
validation:
  - go test ./internal/tasks ./internal/gitops
created_by: codex
---

# Task: Add pack push integration branch

## Goal

Push the final integration branch after a successful convergence merge.

## Required behavior

- push the final integration branch to `origin`
- save push output as an artifact
- fail the publish step if push fails

## Done when

The pack can publish one converged branch instead of leaving task work spread across multiple local branches.
