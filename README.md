# Ptolemy

Ptolemy is a local worker and MCP execution platform for agent-driven coding workflows. It gives an assistant a controlled runtime for opening sessions, reading and editing files, running commands through tmux, tracking execution in SQLite, working with Git, and isolating tasks in worktrees.

The project is intentionally local-first: Codex or another planner decides what should happen, while Ptolemy performs deterministic workspace operations and records what happened.

## What It Does

- Runs a local HTTP worker daemon (`workerd`).
- Creates persistent workspace-bound sessions.
- Executes commands through tmux-backed runners.
- Provides file read, write, list, search, and basic patch operations.
- Exposes Git status, diff, log, checkout, branch, commit, and push helpers.
- Creates isolated Git worktrees for safer parallel task work.
- Stores execution memory in SQLite.
- Stores agent-readable project knowledge in Markdown.
- Exposes the worker through an MCP adapter (`ptolemy-mcp`).
- Includes prototypes for a local LLM executor (`ptolemy-agent`) and task queue runner (`ptolemy-task-runner`).
- Builds deterministic execution plans from task metadata.
- Validates task files before sequential execution.
- Supports CLI plan and run commands for inbox task workflows.

## Architecture

```text
Codex / MCP client / local agent
        |
        v
ptolemy-mcp (optional JSON-RPC stdio adapter)
        |
        v
workerd HTTP API
        |
        +-- sessions and command logs -> SQLite
        +-- command execution -> tmux
        +-- file operations -> workspace filesystem
        +-- git operations -> repository/worktrees
        +-- navigator context -> .ptolemy + docs memory
```

Ptolemy uses two kinds of memory:

- SQLite execution memory for sessions, command logs, actions, logs, and approvals
- Markdown knowledge memory for architecture notes, conventions, decisions, and known issues

For deeper design notes, see [Architecture](./docs/Architecture.md) and [Project Memory](./docs/memory/projects/ptolemy).

## Repository Layout

```text
cmd/
  workerd/              HTTP worker daemon
  ptolemy-mcp/          MCP adapter for the worker API
  ptolemy-agent/        local LLM-driven executor prototype
  ptolemy-task-runner/  markdown queue runner and task planning CLI

internal/
  action/ approval/ logs/ store/   SQLite execution memory
  command/ terminal/ executor/     command execution path
  fileops/ navigator/ memory/      workspace and context tools
  gitops/ worktree/                Git and isolation helpers
  httpapi/                         HTTP routes
  mcp/                             MCP tool definitions and JSON-RPC server
  brain/ worker/                   clients for local LLM and worker APIs
  inspect/ policy/                 workspace inspection and command policy

docs/
  Architecture.md
  memory/
  tasks/
  workflows/
```

## Docs

Core docs are split into focused entry points:

- [Documentation Hub](./docs/README.md)
- [Setup](./docs/Setup.md)
- [CLI Guide](./docs/CLI.md)
- [Worker API](./docs/Worker_API.md)
- [Development Workflow](./docs/Development.md)

## Task System

Tasks live under [`docs/tasks`](./docs/tasks), and the system is built around small, bounded work items with explicit metadata and file scope. For a single isolated change, a loose task file is enough. For anything that needs shared context, reusable snippets, or multiple related task files, use a task pack.

Task packs are the best way to model multi-step work because they keep planning, inputs, and runnable tasks together in one place:

```text
docs/tasks/packs/<pack-name>/
├── PACK_MANIFEST.yaml
├── README.md
├── TASK_PLAN.md
├── inbox/
│   ├── 01-*.md
│   ├── 02-*.md
│   └── 99-final-validation.md
├── scripts/
├── snippets/
└── task-scripts/
```

What a pack gives you:

- One shared plan in `TASK_PLAN.md`
- Pack-level metadata in `PACK_MANIFEST.yaml`
- Runnable task files in `inbox/`
- Reusable references in `snippets/` and `task-scripts/`
- Optional helper assets in `scripts/`

In v1, Ptolemy executes a pack directly from its folder, validates referenced assets, and runs the pack `inbox/` tasks in dependency order. It does not automatically execute pack shell hooks in `scripts/`.

Pack commands:

```bash
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

See [Task System Overview](./docs/tasks/README.md), [Task-File Driven Workflow](./docs/workflows/agent/task-file-driven.md), and example packs in [`docs/tasks/packs`](./docs/tasks/packs).

## [Workflow System](./WORKFLOWS.md)

Ptolemy workflows exist so agents do not improvise the execution model on every task. The workflow system defines the safe, repeatable path for reading context, selecting tools, editing files, recovering from worker drops, and committing changes.

Why workflows matter:

- They keep execution deterministic instead of prompt-driven
- They tell the agent what to read first and what to skip
- They separate task execution, editing, recovery, and Git safety into focused docs
- They reduce broad rewrites by favoring targeted, observable steps

`WORKFLOWS.md` is the index entry point. An agent reads it first, then opens only the workflow document needed for the current task.

Workflow docs are grouped by purpose:

```text
docs/tasks/inbox
docs/tasks/active
docs/tasks/process
docs/tasks/split
docs/tasks/done
docs/tasks/failed
docs/tasks/archive
```

Run the task runner:

```bash
go run ./cmd/ptolemy-task-runner
```

## Task System

Tasks live in `docs/tasks/` and are intended to describe one bounded change. Root task files use the naming format `<Priority>-<task-slug>.md`, where the priority prefix is `Urgent`, `Normal`, or `Low`.

Each task starts with YAML metadata such as `priority`, `task_id`, `parent_task`, `owner`, `status`, `branch`, `allowed_files`, and `created_by`. Agents work on one `task_id` per session, use the declared task branch, and only edit paths listed in `allowed_files`.

Split tasks inherit the parent priority, parent task ID, and allowed file scope unless a child task narrows it. Each child receives its own unique `task_id` and branch.

Task templates:

- `docs/tasks/templates/task-file-template.md`
- `docs/tasks/templates/split-task-template.md`

## Workflow System

`WORKFLOWS.md` is the workflow index. Agents read it first, then load only the workflow file relevant to the current task.

Workflow documents are split by purpose:

```text
docs/workflows/core/
docs/workflows/agent/
docs/workflows/editing/
docs/workflows/recovery/
docs/workflows/git/
```

This keeps context small while still documenting command execution, task-file handling, editing, recovery, safe commits, task branches, and pull requests.

## Git And Pull Requests

Task work happens on the branch declared by task metadata, usually `ptolemy/<priority>-<task-id>`. Stage explicit task files only, never use `git add .`, and commit task-related changes on the task branch after validation.

The pull request workflow is: push the branch, create a Pull Request with the GitHub CLI when available, and write fallback instructions under `.state/pr/` if the CLI is unavailable or unauthenticated. Do not auto-merge unless a task explicitly requests it.

## Development Workflow

Before editing behavior:

```bash
git status --short
go test ./...
```

For normal changes:

```bash
go fmt ./...
go test ./...
git diff --stat
git diff --name-only
```

Project conventions:

- Search first, read small, edit targeted, test immediately.
- Keep command execution behind the runner; handlers should not shell out directly.
- Prefer structured JSON input and output for APIs.
- Keep reusable agent knowledge in Markdown, not hidden in prompts.
- Do not commit `.state/`, `state/*.db`, `bin/`, or temporary `tmp-*.txt` files.
- Never push without explicit approval.

## Current Status

Completed or mostly complete:

- Worker daemon and health check.
- Session persistence and recovery.
- tmux-backed command execution.
- File operations with workspace path restrictions.
- MCP adapter and core tool exposure.
- Git endpoints and MCP tools.
- Worktree creation, listing, removal, and session binding.
- SQLite execution memory tables and migrations.
- Markdown knowledge memory structure.
- Basic local-brain agent loop and task runner prototype.
- Split workflow documentation, task metadata rules, and safe commit/PR guidance.

Still in progress:

- Full approval flow for dangerous actions.
- More complete policy hardening.
- Failure recovery in the agent loop.
- Short command-output summaries.
- Full Codex bridge service.
- End-to-end task execution, validation, and queue finalization.

See `docs/Worker_Progress_Checklist.md` for the detailed phase checklist.

High-signal workflow highlights:

- `core/` covers worker health, sessions, command execution, terminal runners, and worktrees
- `agent/` explains navigator usage, file search/read flow, task-file execution, and planner vs executor boundaries
- `editing/` documents marker-based edits and patch conventions for small, safe changes
- `recovery/` covers EOF or invalid multi-action failures without blindly restarting work
- `git/` defines safe commit behavior, including explicit staging and verification

Start with [Workflow Index](./WORKFLOWS.md), then drill into [workflow docs](./docs/workflows) for the implementation details.

## Design Principles

```text
Deterministic over smart.
File-based over prompt-based.
Search before read.
Safe edits over broad rewrites.
Local-first execution.
Agent-compatible architecture.
```

## More Documentation

- [docs/README.md](./docs/README.md) is the main documentation hub
- [WORKFLOWS.md](./WORKFLOWS.md) indexes supported execution workflows
- [docs/workflows](./docs/workflows) contains focused workflow files for core runtime, agent operation, editing, recovery, and Git safety
- [docs/tasks](./docs/tasks) contains the task system docs and pack examples
- [docs/plans/MVP_Design.md](./docs/plans/MVP_Design.md) describes the planner/executor/runtime model
- [docs/plans/Build Plan.md](./docs/plans/Build%20Plan.md) lays out the build phases
- [docs/plans/Future Updates.md](./docs/plans/Future%20Updates.md) lists future MCP, infrastructure, and safety ideas
- [docs/memory/projects/ptolemy](./docs/memory/projects/ptolemy) contains agent-readable architecture, conventions, decisions, and known issues
