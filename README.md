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

- SQLite execution memory for sessions, command logs, actions, logs, and approvals.
- Markdown knowledge memory for architecture notes, conventions, decisions, and known issues.

See `docs/Architecture.md` and `docs/memory/projects/ptolemy/` for the deeper design notes.

## Repository Layout

```text
cmd/
  workerd/              HTTP worker daemon
  ptolemy-mcp/          MCP adapter for the worker API
  ptolemy-agent/        local LLM-driven executor prototype
  ptolemy-task-runner/  markdown task queue runner prototype

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

## Requirements

- Go 1.25 or newer, matching `go.mod`.
- Make.
- tmux, for command execution.
- ripgrep (`rg`), for code search features.
- Git.
- Optional: `jq` for smoke-test output formatting.
- Optional: llama.cpp server for `ptolemy-agent` local brain mode.

## Setup

```bash
cp .env.example .env
go mod tidy
```

Default environment values:

```env
APP_ENV=development
HTTP_PORT=8080
LOG_LEVEL=debug
STATE_DIR=./state
DB_PATH=./state/ptolemy.db
```

## Common Commands

```bash
make run         # run workerd
make build       # build bin/workerd
make build-mcp   # build bin/ptolemy-mcp
make test        # run go test ./...
make fmt         # run go fmt ./...
make tidy        # run go mod tidy
```

You can also run commands directly:

```bash
go run ./cmd/workerd
go run ./cmd/ptolemy-task-runner
go run ./cmd/ptolemy-agent --task-file docs/tasks/<task>.md --max-steps 8
```

## Running The Worker

Start the HTTP worker:

```bash
make run
```

Check that it is alive:

```bash
curl -s http://localhost:8080/health | jq
```

Expected shape:

```json
{
  "status": "ok",
  "service": "workerd",
  "timestamp": "..."
}
```

## Worker API

The HTTP API is implemented in `internal/httpapi`.

| Area | Endpoints |
|---|---|
| Health | `GET /health` |
| Sessions | `POST /sessions`, `GET /sessions`, `GET /sessions/{id}`, `POST /sessions/{id}/close` |
| Commands | `POST /sessions/{id}/commands` |
| Executor | `POST /execute` |
| Files | `POST /file/read`, `/file/write`, `/file/list`, `/file/search`, `/file/apply` |
| Navigator | `POST /navigator/index`, `/navigator/context`, `/navigator/session/start`, `/navigator/session/note` |
| Git | `POST /git/status`, `/git/diff`, `/git/log`, `/git/checkout`, `/git/branch`, `/git/commit`, `/git/push` |
| Worktrees | `POST /worktree/create`, `/worktree/list`, `/worktree/remove` |

Create a session:

```bash
SESSION_ID=$(curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"name":"local-test","workspace":"'"$PWD"'"}' | jq -r .id)
```

Run a command through the higher-level executor:

```bash
curl -s -X POST http://localhost:8080/execute \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"'"$SESSION_ID"'",
    "command":"echo hello from ptolemy",
    "cwd":"'"$PWD"'",
    "reason":"smoke test",
    "timeout":30
  }' | jq
```

Read a file through the worker:

```bash
curl -s -X POST http://localhost:8080/file/read \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"'"$SESSION_ID"'","path":"README.md"}' | jq
```

## MCP Adapter

`ptolemy-mcp` exposes the worker API as MCP tools over JSON-RPC stdio.

Build it:

```bash
make build-mcp
```

Run it against the default worker URL:

```bash
./bin/ptolemy-mcp
```

Override the worker URL when needed:

```bash
PTOLEMY_WORKER_URL=http://localhost:8080 ./bin/ptolemy-mcp
```

Exposed MCP tool groups include:

- `ptolemy.create_session`, `ptolemy.list_sessions`, `ptolemy.get_session`, `ptolemy.close_session`
- `ptolemy.execute`
- `ptolemy.read_file`, `ptolemy.write_file`, `ptolemy.list_directory`, `ptolemy.search_codebase`, `ptolemy.apply_patch`
- `ptolemy.index_workspace`, `ptolemy.read_context`, `ptolemy.start_task_session`, `ptolemy.append_session_note`
- `ptolemy.git_status`, `ptolemy.git_diff`, `ptolemy.git_log`, `ptolemy.git_checkout`, `ptolemy.git_create_branch`, `ptolemy.git_commit`, `ptolemy.git_push`
- `ptolemy.create_worktree`, `ptolemy.list_worktrees`, `ptolemy.remove_worktree`

## Agent And Task Runner

`ptolemy-agent` is a local executor loop. It asks a local llama.cpp-compatible model for exactly one JSON action at a time, then performs that action through `workerd`.

Supported action types include:

- `run_command`
- `read_file`
- `write_file`
- `replace_block`
- `insert_after`
- `explain`
- `ask_approval`

Run a task file:

```bash
go run ./cmd/ptolemy-agent --task-file docs/tasks/<task>.md --max-steps 8
```

Allow script creation or execution only when the task explicitly needs it:

```bash
go run ./cmd/ptolemy-agent --allow-scripts --task-file docs/tasks/<task>.md --max-steps 3
```

`ptolemy-task-runner` scans Markdown tasks under `docs/tasks`, classifies task size, moves work through active/done/failed queues, and invokes `ptolemy-agent` with an appropriate step budget.

Task queues:

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

- `WORKFLOWS.md` indexes supported execution workflows.
- `docs/workflows/` contains focused workflow files for core runtime, agent operation, editing, recovery, Git, and Pull Request handling.
- `docs/tasks/templates/` contains root and split task templates.
- `docs/MVP_Design.md` describes the planner/executor/runtime model.
- `docs/Build Plan.md` lays out the build phases.
- `docs/Future Updates.md` lists future MCP, infrastructure, and safety ideas.
- `docs/memory/projects/ptolemy/` contains agent-readable architecture, conventions, decisions, and known issues.
