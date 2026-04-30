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
| Files | `POST /file/read`, `/file/write`, `/file/list`, `/file/search`, `/file/apply` |
| Navigator | `POST /navigator/index`, `/navigator/context`, `/navigator/session/start`, `/navigator/session/note` |
| Git | `POST /git/status`, `/git/diff`, `/git/log`, `/git/checkout`, `/git/branch`, `/git/commit`, `/git/push` |
| Worktrees | `POST /worktree/create`, `/worktree/list`, `/worktree/remove` |
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

## Example: Read A File

```bash
curl -s -X POST http://localhost:8080/file/read \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"'"$SESSION_ID"'","path":"README.md"}' | jq
```

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
- `ptolemy.read_file`, `ptolemy.write_file`, `ptolemy.list_directory`, `ptolemy.search_codebase`, `ptolemy.apply_patch`
- `ptolemy.index_workspace`, `ptolemy.read_context`, `ptolemy.start_task_session`, `ptolemy.append_session_note`
- `ptolemy.git_status`, `ptolemy.git_diff`, `ptolemy.git_log`, `ptolemy.git_checkout`, `ptolemy.git_create_branch`, `ptolemy.git_commit`, `ptolemy.git_push`
- `ptolemy.create_worktree`, `ptolemy.list_worktrees`, `ptolemy.remove_worktree`
