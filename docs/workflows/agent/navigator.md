# Navigator Workflow

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
