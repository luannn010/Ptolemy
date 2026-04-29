# Command Execution Workflows

## Command Execution Workflow

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

## Execute Endpoint Workflow

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
