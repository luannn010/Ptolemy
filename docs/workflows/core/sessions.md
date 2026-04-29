# Session Workflows

## Session Workflow

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

## Persistent Session Workflow

Sessions are stored in SQLite and can be reused after worker restart.

```text
Worker starts
  -> Uses configured DB_PATH
  -> Existing sessions remain available
  -> Sessions can be listed, fetched, or closed
```

Status: working when the restarted worker points at the same `DB_PATH`.
