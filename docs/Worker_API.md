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
PTOLEMY_BASE_URL=http://127.0.0.1:8080 ./bin/ptolemy-mcp
```

This adapter speaks MCP STDIO framing with `Content-Length` headers and exposes:

- `ptolemy_health` -> `GET /health`
- `ptolemy_create_session` -> `POST /sessions/`
- `ptolemy_execute` -> `POST /execute`
- `ptolemy_run_task_file` -> best-effort `POST /agent/run`

If `POST /agent/run` is not available on the worker, the adapter returns a clean fallback response instead of hiding the failure.

Codex Custom MCP UI values:

- Name: `Ptolemy`
- Type: `STDIO`
- Command: `./bin/ptolemy-mcp.exe`
- Arguments: none
- Working directory: repo root
- Environment variable: `PTOLEMY_BASE_URL=http://<tailscale-ip>:8080`

Smoke test:

```bash
GOOS=windows GOARCH=amd64 go build -o bin/ptolemy-mcp.exe ./cmd/ptolemy-mcp
curl -s http://<tailscale-ip>:8080/health | jq
```
