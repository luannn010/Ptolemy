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

For deeper design notes, see [docs/Architecture.md](/home/luannn010/projects/ptolemy/docs/Architecture.md) and [docs/memory/projects/ptolemy](/home/luannn010/projects/ptolemy/docs/memory/projects/ptolemy).

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

Core docs are now split into smaller files:

- [Documentation Hub](/home/luannn010/projects/ptolemy/docs/README.md)
- [Setup](/home/luannn010/projects/ptolemy/docs/Setup.md)
- [CLI Guide](/home/luannn010/projects/ptolemy/docs/CLI.md)
- [Worker API](/home/luannn010/projects/ptolemy/docs/Worker_API.md)
- [Development Workflow](/home/luannn010/projects/ptolemy/docs/Development.md)

## Task System

Tasks live in `docs/tasks/` and are intended to describe one bounded change. Root task files use the naming format `<Priority>-<task-slug>.md`, where the priority prefix is `Urgent`, `Normal`, or `Low`.

Each task starts with YAML metadata such as `priority`, `task_id`, `parent_task`, `owner`, `status`, `branch`, `allowed_files`, and `created_by`. Agents work on one `task_id` per session, use the declared task branch, and only edit paths listed in `allowed_files`.

Split tasks inherit the parent priority, parent task ID, and allowed file scope unless a child task narrows it. Each child receives its own unique `task_id` and branch.

Task templates:

- `docs/tasks/templates/task-file-template.md`
- `docs/tasks/templates/split-task-template.md`
- `docs/tasks/templates/task-pack-template/`

Use a loose task file for one bounded change with no shared task assets. Use a task pack when multiple tasks need a shared execution plan, shared snippets, or shared task-script references.

Task packs contain:

- `TASK_PLAN.md`
- `PACK_MANIFEST.yaml`
- `README.md`
- `scripts/`
- `task-scripts/`
- `snippets/`
- `inbox/`

Pack `inbox/` tasks run directly from the pack directory in v1 through:

```bash
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

Pack assets are validated references only in v1. The runner verifies referenced `task-scripts/` and `snippets/` files exist, but it does not auto-run pack shell hooks under `scripts/`.

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

See [WORKFLOWS.md](/home/luannn010/projects/ptolemy/WORKFLOWS.md) and [docs/workflows](/home/luannn010/projects/ptolemy/docs/workflows).

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

- [docs/README.md](/home/luannn010/projects/ptolemy/docs/README.md) is the main documentation hub
- [WORKFLOWS.md](/home/luannn010/projects/ptolemy/WORKFLOWS.md) indexes supported execution workflows
- [docs/workflows](/home/luannn010/projects/ptolemy/docs/workflows) contains focused workflow files for core runtime, agent operation, editing, recovery, Git, and Pull Request handling
- [docs/tasks/templates](/home/luannn010/projects/ptolemy/docs/tasks/templates) contains task templates, including task packs
- [docs/plans/MVP_Design.md](/home/luannn010/projects/ptolemy/docs/plans/MVP_Design.md) describes the planner/executor/runtime model
- [docs/plans/Build Plan.md](/home/luannn010/projects/ptolemy/docs/plans/Build%20Plan.md) lays out the build phases
- [docs/plans/Future Updates.md](/home/luannn010/projects/ptolemy/docs/plans/Future%20Updates.md) lists future MCP, infrastructure, and safety ideas
- [docs/memory/projects/ptolemy](/home/luannn010/projects/ptolemy/docs/memory/projects/ptolemy) contains agent-readable architecture, conventions, decisions, and known issues
