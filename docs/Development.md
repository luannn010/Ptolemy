# Development Workflow

## Before Editing

```bash
git status --short
go test -p 1 ./...
```

## Normal Change Loop

```bash
go fmt ./...
go test -p 1 ./...
git diff --stat
git diff --name-only
```

## Project Conventions

- Search first, read small, edit targeted, test immediately
- Keep command execution behind the runner; handlers should not shell out directly
- Prefer structured JSON input and output for APIs
- Keep reusable agent knowledge in Markdown, not hidden in prompts
- Do not commit `.state/`, `state/*.db`, `bin/`, or temporary `tmp-*.txt` files
- Never push without explicit approval

## Git And Pull Requests

Task work happens on the branch declared by task metadata, usually `ptolemy/<priority>-<task-id>`.

- Stage explicit task files only
- Never use `git add .`
- Commit task-related changes after validation
- Do not auto-merge unless a task explicitly requests it

For the commit workflow, see [Safe Commit](./workflows/git/safe-commit.md).

## Current Status

Completed or mostly complete:

- Worker daemon and health check
- Session persistence and recovery
- tmux-backed command execution
- File operations with workspace path restrictions
- MCP adapter and core tool exposure
- Git endpoints and MCP tools
- Worktree creation, listing, removal, and session binding
- SQLite execution memory tables and migrations
- Markdown knowledge memory structure
- Basic local-brain agent loop and task runner prototype
- Split workflow documentation, task metadata rules, and safe commit/PR guidance
- Deterministic task planning, validation, sequential scheduling, and CLI preview/run commands

Still in progress:

- Full approval flow for dangerous actions
- More complete policy hardening
- Failure recovery in the agent loop
- Short command-output summaries
- Full Codex bridge service
- End-to-end task execution, validation, and queue finalization

See [Worker Progress Checklist](./plans/Worker_Progress_Checklist.md) for the full checklist.
