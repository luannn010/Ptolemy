---
project: Ptolemy
category: code-map
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - code-map
---

# Package Map

## `internal/action`

Responsibility:

Action records, metadata, validation helpers, and splitter interfaces.

Important files:

`store.go`, `metadata.go`, `validator.go`, `splitter.go`

Important functions/types:

`Store`, `Action`, `ActionEnvelope`

Used by:

`internal/executor`, `internal/agentloop`, `internal/httpapi`

Status:

Ready

## `internal/agentloop`

Responsibility:

Controller-driven agent runs, observations, tools, reasoning profiles, and finalization.

Important files:

`service.go`, `store.go`, `tool_executor.go`, `reasoning_profile.go`, `finalize.go`

Important functions/types:

`Service`, `Store`, `ToolRegistry`, `ReasoningProfile`

Used by:

`cmd/workerd`, `internal/httpapi`, `internal/packstudio`

Status:

Partial

## `internal/approval`

Responsibility:

Approval persistence for risky actions.

Important files:

`store.go`

Important functions/types:

`Store`, `Approval`

Used by:

`internal/httpapi`, `internal/agentloop`

Status:

Partial

## `internal/brain`

Responsibility:

OpenAI-compatible local brain client.

Important files:

`client.go`

Important functions/types:

`Client`, `Message`

Used by:

`cmd/ptolemy-agent`, `cmd/workerd`

Status:

Partial

## `internal/command`

Responsibility:

Command log persistence and request models.

Important files:

`store.go`, `command.go`

Important functions/types:

`Store`, `CommandLog`, `RunCommandRequest`

Used by:

`internal/httpapi`, `internal/executor`, `internal/packstudio`

Status:

Ready

## `internal/config`

Responsibility:

Runtime configuration loading from environment.

Important files:

`config.go`

Important functions/types:

`Config`, `Load`

Used by:

`cmd/workerd`, `cmd/ptolemy-mcp`, `cmd/ptolemy-agent`, `cmd/ptolemy-task-runner`

Status:

Ready

## `internal/executor`

Responsibility:

High-level execution orchestration for `/execute`.

Important files:

`executor.go`

Important functions/types:

`Executor`, `ExecuteRequest`, `ExecuteResponse`

Used by:

`internal/httpapi`

Status:

Ready

## `internal/fileops`

Responsibility:

Workspace file read/write/list/search/apply helpers.

Important files:

`fileops.go`

Important functions/types:

`FileOps`

Used by:

`internal/httpapi`

Status:

Ready

## `internal/gitops`

Responsibility:

Git status, diff, log, branch, commit, and push helpers.

Important files:

`gitops.go`

Important functions/types:

`GitOps`

Used by:

`internal/httpapi`

Status:

Ready

## `internal/httpapi`

Responsibility:

HTTP router and handlers for runtime, task, KB, agent, Git, worktree, and UI operations.

Important files:

`router.go`, `session.go`, `commands.go`, `execute.go`, `fileops.go`, `agent_runs.go`, `packstudio.go`

Important functions/types:

`NewRouter`

Used by:

`cmd/workerd`

Status:

Ready

## `internal/inspect`

Responsibility:

Workspace inspection and snapshotting for agent prompts.

Important files:

`inspect.go`

Important functions/types:

`InspectWorkspace`

Used by:

`cmd/ptolemy-agent`

Status:

Partial

## `internal/logging`

Responsibility:

Structured runtime log persistence and zerolog setup.

Important files:

`logging.go`, `store.go`

Important functions/types:

`Setup`, `Store`

Used by:

`cmd/workerd`, `internal/httpapi`, `internal/executor`

Status:

Partial

## `internal/mcp`

Responsibility:

MCP server, worker client, tool declarations, and tool handlers.

Important files:

`server.go`, `client.go`, `tools.go`, `rpc.go`, plus `*tools/tools.go`

Important functions/types:

`Server`, `WorkerClient`, `Tool`

Used by:

`cmd/ptolemy-mcp`

Status:

Ready

## `internal/memory`

Responsibility:

Load KB, compatibility context, or docs memory markdown.

Important files:

`loader.go`

Important functions/types:

`LoadWorkspaceMemory`

Used by:

`internal/httpapi/commands.go`

Status:

Partial

## `internal/navigator`

Responsibility:

KB indexing, compatibility context, task-session helpers, and project-map generation.

Important files:

`kb.go`, `navigator.go`, `context_budget.go`

Important functions/types:

`IndexWorkspace`, `ReadContext`, `UpdateKnowledgeBase`

Used by:

`internal/httpapi`, `cmd/ptolemy-agent`

Status:

Partial

## `internal/packstudio`

Responsibility:

Program, pack, and run orchestration backing the `/ui` experience.

Important files:

`service.go`, `definitions.go`, `store.go`, `types.go`

Important functions/types:

`Service`, `CreateProgramRun`, `ProgramRun`

Used by:

`cmd/workerd`, `internal/httpapi`

Status:

Partial

## `internal/policy`

Responsibility:

Simple allow/ask/deny command policy rules.

Important files:

`policy.go`

Important functions/types:

`CheckCommand`, `Decision`

Used by:

`internal/httpapi/commands.go`

Status:

Partial

## `internal/session`

Responsibility:

Session model and persistence.

Important files:

`session.go`, `store.go`

Important functions/types:

`Store`, `CreateSessionRequest`, `Session`

Used by:

Most runtime packages

Status:

Ready

## `internal/shellcmd`

Responsibility:

OS-aware shell command helpers.

Important files:

`shellcmd.go`

Important functions/types:

OS-aware shell helpers

Used by:

Command execution path and tests

Status:

Partial

## `internal/store`

Responsibility:

SQLite base store and migrations.

Important files:

`store.go`, `migrations.go`

Important functions/types:

`Open`, `RunMigrations`

Used by:

`cmd/workerd`, `cmd/ptolemy-agent`, store-backed packages

Status:

Ready

## `internal/tasks`

Responsibility:

Task scanning, validation, planning, scheduling, process state, packs, and bootstrap support.

Important files:

`scanner.go`, `validator.go`, `planner.go`, `scheduler.go`, `pack.go`, `process.go`, `bootstrap.go`

Important functions/types:

`ScanInbox`, `RunInboxScheduler`, `LoadTaskPack`, `RunTaskPack`

Used by:

`cmd/ptolemy-task-runner`, `internal/httpapi`, `internal/agentloop`, `internal/packstudio`

Status:

Partial

## `internal/terminal`

Responsibility:

tmux-backed and fallback command execution.

Important files:

`tmux_runner.go`, `runner.go`

Important functions/types:

`TmuxRunner`, `Runner`

Used by:

`cmd/workerd`, `internal/executor`, `internal/httpapi`, `internal/packstudio`

Status:

Partial

## `internal/worker`

Responsibility:

HTTP client for `workerd`.

Important files:

`client.go`

Important functions/types:

`Client`

Used by:

`cmd/ptolemy-agent`, `cmd/ptolemy-task-runner`

Status:

Ready

## `internal/worktree`

Responsibility:

Git worktree management and isolation.

Important files:

`worktree.go`

Important functions/types:

`Manager`

Used by:

`internal/httpapi`

Status:

Partial

