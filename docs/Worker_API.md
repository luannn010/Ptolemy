# Worker API

The HTTP API is implemented in `internal/httpapi`.

## Health Check

Start the worker:

```bash
make run
```

Check health:

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

## Endpoint Areas

| Area | Endpoints |
|---|---|
| Health | `GET /health` |
| Sessions | `POST /sessions`, `GET /sessions`, `GET /sessions/{id}`, `POST /sessions/{id}/close` |
| Commands | `POST /sessions/{id}/commands` |
| Executor | `POST /execute` |
| Files | `POST /file/read`, `/file/write`, `/file/list`, `/file/search`, `/file/apply`, `/file/replace_block`, `/file/insert_after` |
| Navigator | `POST /navigator/index`, `/navigator/context`, `/navigator/session/start`, `/navigator/session/note` |
| KB | `POST /kb/build`, `/kb/read`, `/kb/update` |
| Git | `POST /git/status`, `/git/diff`, `/git/log`, `/git/checkout`, `/git/branch`, `/git/commit`, `/git/push` |
| Worktrees | `POST /worktree/create`, `/worktree/list`, `/worktree/remove` |
| Agent Runs | `POST /agent-runs`, `GET /agent-runs/{id}`, `GET /agent-runs/{id}/actions`, `GET /agent-runs/{id}/observations`, `POST /agent-runs/{id}/resume`, `POST /agent-runs/{id}/cancel` |
| Tasks | `POST /tasks/run-inbox` |

## Example: Create A Session

```bash
SESSION_ID=$(curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"name":"local-test","workspace":"'"$PWD"'"}' | jq -r .id)
```

## Example: Execute A Command

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

Optional descriptive metadata fields are also accepted on `/execute` and `ptolemy.execute`:

- `title`
- `purpose`
- `reasoning_step`
- `target`

## Example: Read A File

```bash
curl -s -X POST http://localhost:8080/file/read \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"'"$SESSION_ID"'","path":"README.md"}' | jq
```

## Example: Targeted File Edits

Use targeted edits when an agent should avoid rewriting a whole file.

Replace the first exact text block match:

```bash
curl -s -X POST http://localhost:8080/file/replace_block \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"'"$SESSION_ID"'",
    "path":"README.md",
    "old":"old exact text",
    "new":"new exact text"
  }' | jq
```

Insert content after the first exact marker match:

```bash
curl -s -X POST http://localhost:8080/file/insert_after \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"'"$SESSION_ID"'",
    "path":"README.md",
    "marker":"<!-- PTOLEMY: INSERT HERE -->",
    "content":"inserted text"
  }' | jq
```

`insert_after` prepends a newline to `content` when one is not already present.

## Example: Create An Agent Run

This creates a controller-owned run record without auto-starting the loop:

```bash
curl -s -X POST http://localhost:8080/agent-runs \
  -H 'Content-Type: application/json' \
  -d '{
    "task_id":"smoke-agentloop",
    "workspace":"'"$PWD"'",
    "branch":"ptolemy/smoke-agentloop",
    "max_steps":4,
    "current_phase":"manual-smoke",
    "auto_start":false
  }' | jq
```

## Example: Start The Controller Loop

When `auto_start` is `true`, `workerd` loads task context, asks the model for one JSON action at a time, validates that action, executes it, records observations, and loops until completion or guardrails stop the run.

```bash
curl -s -X POST http://localhost:8080/agent-runs \
  -H 'Content-Type: application/json' \
  -d '{
    "task_id":"my-task",
    "task_file":"docs/tasks/inbox/Normal-my-task.md",
    "workspace":"'"$PWD"'",
    "branch":"ptolemy/normal-my-task",
    "max_steps":8,
    "current_phase":"task_runner",
    "auto_start":true
  }' | jq
```

## Reasoning Loop Contract

The agent loop is controller-driven:

1. The model proposes exactly one top-level JSON action.
2. `workerd` validates the action and rejects invalid or multi-object output.
3. `workerd` executes the chosen tool.
4. The tool result is persisted as an observation.
5. The loop continues until completion, pause, cancellation, or guardrail failure.

The model does not directly execute shell, file, Git, or publish operations.

## MCP Adapter

Build the adapter:

```bash
make build-mcp
```

Run it:

```bash
./bin/ptolemy-mcp
```

Override the worker URL when needed:

```bash
PTOLEMY_WORKER_URL=http://localhost:8080 ./bin/ptolemy-mcp
```

Exposed MCP groups include:

- `ptolemy.create_session`, `ptolemy.list_sessions`, `ptolemy.get_session`, `ptolemy.close_session`
- `ptolemy.execute`
- `ptolemy.read_file`, `ptolemy.write_file`, `ptolemy.list_directory`, `ptolemy.search_codebase`, `ptolemy.apply_patch`, `ptolemy.replace_block`, `ptolemy.insert_after`
- `ptolemy.index_workspace`, `ptolemy.read_context`, `ptolemy.start_task_session`, `ptolemy.append_session_note`
- `ptolemy.kb_build`, `ptolemy.kb_read`, `ptolemy.kb_update`
- `ptolemy.git_status`, `ptolemy.git_diff`, `ptolemy.git_log`, `ptolemy.git_checkout`, `ptolemy.git_create_branch`, `ptolemy.git_commit`, `ptolemy.git_push`
- `ptolemy.create_worktree`, `ptolemy.list_worktrees`, `ptolemy.remove_worktree`
