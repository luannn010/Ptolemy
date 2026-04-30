---
priority: normal
task_id: add-pack-success-pr-create
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-success-pr-create
execution_group: sequential
depends_on:
  - add-pack-integration-branch-merge
  - add-pack-push-integration-branch
allowed_files:
  - internal/tasks/pack_runtime.go
  - internal/tasks/pack_test.go
  - internal/gitops/gitops.go
validation:
  - go test ./internal/tasks ./internal/gitops
created_by: codex
---

# Task: Add pack success pull request create

## Goal

Create a pull request from the converged integration branch to `main`.

## Required behavior

- create the pull request after the branch push succeeds
- use the generated PR draft content as the body
- save PR creation output as an artifact

## Done when

The pack can finish by opening one pull request to merge the converged branch into `main`.
