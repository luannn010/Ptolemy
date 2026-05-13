# Worktree Workflow

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
