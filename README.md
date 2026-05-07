# Ptolemy

Ptolemy is a local worker and MCP execution platform for agent-driven coding workflows. It gives an assistant a controlled runtime for opening sessions, reading and editing files, running commands through tmux, tracking execution in SQLite, working with Git, and isolating tasks in worktrees.

The project is intentionally local-first: Codex or another planner decides what should happen, while Ptolemy performs deterministic workspace operations and records what happened.

## What It Does

- Runs a local HTTP worker daemon (`workerd`).
- Creates persistent workspace-bound sessions.
- Executes commands through tmux-backed runners.
- Chooses an OS-native shell for generic command execution: PowerShell on Windows and Bash on Unix-like systems.
- Provides file read, write, list, search, and basic patch operations.
- Exposes Git status, diff, log, checkout, branch, commit, and push helpers.
- Creates isolated Git worktrees for safer parallel task work.
- Stores execution memory in SQLite.
- Stores agent-readable project knowledge in a KB-first `.ptolemy/kb` workspace.
- Exposes the worker through an MCP adapter (`ptolemy-mcp`).
- Includes prototypes for a local LLM executor (`ptolemy-agent`) and task queue runner (`ptolemy-task-runner`).
- Builds deterministic execution plans from task metadata.
- Validates task files before sequential execution.
- Supports CLI plan and run commands for inbox task workflows.
- Supports bootstrapping workflow and task scaffolding into another workspace.

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
        +-- navigator KB/context -> .ptolemy + docs memory
```

Ptolemy uses two kinds of memory:

- SQLite execution memory for sessions, command logs, actions, logs, and approvals
- KB-first Markdown and JSON knowledge memory under `.ptolemy/kb` for project maps, file indexes, symbol indexes, workflows, decisions, and changelog history

For deeper design notes, see [Architecture](./docs/Architecture.md) and [Project Memory](./docs/memory/projects/ptolemy).

## KB Workspace

Ptolemy now treats `.ptolemy/kb/` as the canonical repo memory layer for agents. The intended flow is:

1. Read `.ptolemy/PTOLEMY.md`
2. Read `.ptolemy/kb/PROJECT_MAP.md`
3. Use `.ptolemy/kb/FILE_INDEX.json` and `.ptolemy/kb/SYMBOL_INDEX.json` to choose likely files
4. Search and read the repo only after KB triage
5. Update the KB after successful task-pack completion

Canonical KB files:

```text
.ptolemy/
├── PTOLEMY.md
└── kb/
    ├── PROJECT_MAP.md
    ├── FILE_INDEX.json
    ├── SYMBOL_INDEX.json
    ├── WORKFLOWS.md
    ├── DECISIONS.md
    └── CHANGELOG.md
```

Compatibility artifacts are still emitted in MVP for older callers:

- `.ptolemy/index/file-tree.json`
- `.ptolemy/context/*.md`

Generated vs curated KB files:

- Generated or machine-updated: `PROJECT_MAP.md`, `FILE_INDEX.json`, `SYMBOL_INDEX.json`, `CHANGELOG.md`
- Curated: `WORKFLOWS.md`, `DECISIONS.md`

KB surfaces:

```text
POST /navigator/index      compatibility build path
POST /navigator/context    compatibility read path
POST /kb/build             canonical KB build path
POST /kb/read              canonical KB read path
POST /kb/update            canonical KB incremental update path

ptolemy.index_workspace    compatibility MCP tool
ptolemy.read_context       compatibility MCP tool
ptolemy.kb_build           canonical MCP tool
ptolemy.kb_read            canonical MCP tool
ptolemy.kb_update          canonical MCP tool
```

A successful task-pack run updates the KB once after integration merge and before push or PR creation. That update refreshes changed file entries, refreshes Go symbol entries for changed Go files, removes deleted entries, and appends one KB changelog entry for the pack.

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
  shellcmd/                        OS-aware shell selection helpers
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
go run ./cmd/ptolemy-task-runner bootstrap --workspace /path/to/target-repo
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
docs/workflows/core/
docs/workflows/agent/
docs/workflows/editing/
docs/workflows/recovery/
docs/workflows/git/
```

High-signal workflow highlights:

- `core/` covers worker health, sessions, command execution, terminal runners, and worktrees
- `agent/` explains navigator usage, file search/read flow, task-file execution, and planner vs executor boundaries
- `editing/` documents marker-based edits and patch conventions for small, safe changes
- `recovery/` covers EOF or invalid multi-action failures without blindly restarting work
- `git/` defines safe commit behavior, including explicit staging and verification

Start with [Workflow Index](./WORKFLOWS.md), then drill into [workflow docs](./docs/workflows) for the implementation details.

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

## Using Another Workspace

You can point Ptolemy at a different repository without moving the Ptolemy source tree into that workspace.

- Start `workerd` with a reachable `WORKER_BASE_URL`.
- Run `ptolemy-agent --workspace /path/to/repo` so file reads, writes, and commands bind to that repository.
- Configure `BRAIN_BASE_URL` and `BRAIN_MODEL` when your local model endpoint differs from the defaults.
- Put `ptolemy-agent` on `PATH` or set `PTOLEMY_AGENT_BIN` so `ptolemy-task-runner` can invoke the agent binary directly.
- Use `go run ./cmd/ptolemy-task-runner bootstrap --workspace /path/to/repo` to seed `WORKFLOWS.md` and `docs/tasks/templates` into a fresh non-Ptolemy repository.

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
