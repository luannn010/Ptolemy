---
priority: normal
task_id: add-pack-final-validation
parent_task: github-automation-foundation
owner: unassigned
status: inbox
branch: ptolemy/add-pack-final-validation
execution_group: final
depends_on:
  - add-pack-integration-branch-merge
  - add-pack-push-integration-branch
  - add-pack-success-pr-create
  - add-pack-failure-issue-draft
allowed_files:
  - internal/tasks/pack_runtime.go
  - internal/tasks/pack_test.go
  - internal/gitops/gitops.go
  - internal/gitops/gitops_test.go
  - internal/worktree/worktree.go
validation:
  - go test ./internal/tasks ./internal/gitops
created_by: codex
---

# Task: Final validation for pack automation foundation

## Goal

Confirm the integration-branch and publish flow works end to end.

## Required behavior

- merge task branches into one integration branch before publish
- push that integration branch
- open a pull request to `main`
- verify tests cover success and failure paths

## Done when

The first pack automation slice is test-backed and ready for later GitHub API or CLI integration.
