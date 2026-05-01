# Ptolemy Workflows

This document is the workflow index for Ptolemy.

Agents should read this file first, then open only the workflow file relevant to the current task.

## Core Runtime Workflows

- Health Check: `docs/workflows/core/health-check.md`
- Sessions: `docs/workflows/core/sessions.md`
- Command Execution: `docs/workflows/core/command-execution.md`
- Terminal Runner: `docs/workflows/core/terminal-runner.md`
- Worktree: `docs/workflows/core/worktree.md`

## Agent Operation Workflows

- Codex Execution: `docs/workflows/agent/codex-execution.md`
- Task-File Driven Execution: `docs/workflows/agent/task-file-driven.md`
- Task Flags and Isolation: `docs/workflows/agent/task-flags-and-isolation.md`
- Navigator: `docs/workflows/agent/navigator.md`
- File Search / Read: `docs/workflows/agent/file-search-read.md`
- Planner vs Executor: `docs/workflows/agent/planner-vs-executor.md`
- Client-Server Local (No Docker): `docs/workflows/agent/client-server-local.md`

## Editing Workflows

- Marker-Based Editing: `docs/workflows/editing/marker-based-editing.md`
- Patch Spec: `docs/workflows/editing/patch-spec.md`

## Recovery Workflows

- EOF / Worker Drop: `docs/workflows/recovery/eof-worker-drop.md`
- Invalid Multi-Action Recovery: `docs/workflows/recovery/invalid-multi-action.md`

## Git Workflows

- Safe Commit: `docs/workflows/git/safe-commit.md`
- Task Branch and Safe Merge: `docs/workflows/git/task-branch-merge.md`
- Pull Request: `docs/workflows/git/pull-request.md`

## Design Principles

- Deterministic over smart
- File-based over prompt-based
- Search before read
- Safe edits over full rewrites
- Local-first execution
- Agent-compatible architecture
