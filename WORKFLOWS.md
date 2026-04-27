# Ptolemy Workflows

This document defines the core execution and development workflows supported by Ptolemy.
The goal is agent-driven development with deterministic, low-risk operations.

## 1. Health Check Workflow

Verify the worker process is reachable.

```text
Client
  -> GET /health
  -> Worker responds with status/service/timestamp
```

Example:

```bash
curl -s http://localhost:8080/health
```

Status: working.

## 2. Session Workflow

Create and manage isolated execution contexts.

```text
Client / Agent
  -> POST /sessions
  -> Ptolemy stores the session in SQLite
  -> Session binds commands and file tools to a workspace
```

Example:

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"name":"workflow-session","workspace":"/path/to/workspace"}'
```

Status: working.

## 3. Persistent Session Workflow

Sessions are stored in SQLite and can be reused after worker restart.

```text
Worker starts
  -> Uses configured DB_PATH
  -> Existing sessions remain available
  -> Sessions can be listed, fetched, or closed
```

Status: working when the restarted worker points at the same `DB_PATH`.

## 4. Command Execution Workflow

Execute shell commands inside a session workspace.

```text
Client / Agent
  -> POST /sessions/{id}/commands
  -> Validate session and command policy
  -> Execute command through TmuxRunner
  -> Store command/action/log records
  -> Return stdout, stderr, exit code, duration
```

Example:

```bash
curl -s -X POST http://localhost:8080/sessions/<id>/commands \
  -H 'Content-Type: application/json' \
  -d '{"command":"echo hello","timeout":30}'
```

Status: working.

## 5. Execute Endpoint Workflow

Run a command through the higher-level executor API.

```text
Client / Agent
  -> POST /execute
  -> Validate session_id and command
  -> Execute command
  -> Return output, summary, success flag
```

Example:

```bash
curl -s -X POST http://localhost:8080/execute \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"<id>","command":"echo hello","timeout":30}'
```

Status: working.

## 6. Terminal Runner Workflow

Use tmux-backed execution so a session can keep shell state.

```text
HTTP API
  -> Command / Executor service
  -> TmuxRunner
  -> tmux session per Ptolemy session
  -> OS shell
```

Notes:

- Commands that do not print a trailing newline are supported.
- Closing a Ptolemy session kills the matching tmux session.

Status: working.

## 7. Navigator Workflow

Use Ptolemy as a codebase navigator, not a whole-codebase reader.

```text
Agent
  -> POST /navigator/index
  -> Read .ptolemy/PTOLEMY.md and .ptolemy/context/*.md
  -> Search by keyword/symbol
  -> Read only relevant files
  -> Record task notes and files read
```

Index a workspace:

```bash
curl -s -X POST http://localhost:8080/navigator/index \
  -H 'Content-Type: application/json' \
  -d '{"workspace":"/path/to/workspace"}'
```

Read context:

```bash
curl -s -X POST http://localhost:8080/navigator/context \
  -H 'Content-Type: application/json' \
  -d '{"workspace":"/path/to/workspace"}'
```

Start task notes:

```bash
curl -s -X POST http://localhost:8080/navigator/session/start \
  -H 'Content-Type: application/json' \
  -d '{"workspace":"/path/to/workspace","task_session_id":"my-task","task":"Fix the bug"}'
```

Append a note:

```bash
curl -s -X POST http://localhost:8080/navigator/session/note \
  -H 'Content-Type: application/json' \
  -d '{"workspace":"/path/to/workspace","task_session_id":"my-task","note":"Read router and command handler"}'
```

Status: working.

## 8. File Search / Read Workflow

Search before reading full files.

```text
Agent
  -> POST /file/search
  -> Choose top relevant files
  -> POST /file/read with optional task_session_id
  -> Ptolemy records the read in .ptolemy/sessions/<id>/files-read.json
```

Status: working.

## 9. Worktree Workflow

Create isolated Git worktrees per task.

```text
Client / Agent
  -> POST /worktree/create
  -> Validate session
  -> Resolve repo root
  -> Create .ptolemy-worktrees/<name>
  -> Update session workspace to the worktree
```

Example:

```bash
curl -s -X POST http://localhost:8080/worktree/create \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"<id>","name":"my-task","branch":"codex/my-task"}'
```

Status: working.

## 10. Codex Execution Workflow

Ptolemy acts as the execution backend while Codex plans, edits, and validates.

```text
Codex
  -> Reads AGENT.md and .ptolemy context
  -> Uses navigator/search/read tools
  -> Makes targeted edits
  -> Runs commands/tests through Ptolemy
  -> Summarizes results
```

Status: working as a supported development pattern.

## 11. Task-File Driven Workflow

Use structured instructions instead of free-form prompts.

```text
Agent
  -> Reads task file
  -> Classifies scope
  -> Runs ptolemy-agent with a step budget
  -> Stores command output artifacts
```

Current task runner paths:

- `docs/tasks/inbox`
- `docs/tasks/active`
- `docs/tasks/split`
- `docs/tasks/done`
- `docs/tasks/failed`

Status: partially implemented.

## 12. Marker-Based Editing Workflow

Improve reliability of edits by using stable anchors.

```text
Developer or agent inserts a marker
  -> Agent locates marker
  -> Agent uses insert_after
  -> Ptolemy writes the targeted edit
  -> Tests run immediately
```

Example marker:

```go
// PTOLEMY: INSERT ROUTES HERE
```

Status: supported by `ptolemy-agent` insert-after behavior.

## 13. Patch Spec Workflow

Structured patch specs are the intended future replacement for fragile text edits.

Example:

```yaml
type: insert_after
file: cmd/ptolemy-agent/main.go
anchor: "// PTOLEMY: INSERT ACTION CASES HERE"
content: |
  case "insert_after":
```

Status: planned. Basic content replacement exists through file tools, but full patch-spec validation is not implemented yet.

## 14. Planner vs Executor Workflow

Separate reasoning from execution.

```text
Planner
  -> Reads task/context
  -> Produces plan or patch spec

Executor
  -> Validates action
  -> Executes file/command/worktree operations
  -> Returns structured results
```

Status: partially implemented through `ptolemy-agent`, `workerd`, and MCP tools.

## Design Principles

```text
- Deterministic over smart
- File-based over prompt-based
- Search before read
- Safe edits over full rewrites
- Local-first execution
- Agent-compatible architecture
```
