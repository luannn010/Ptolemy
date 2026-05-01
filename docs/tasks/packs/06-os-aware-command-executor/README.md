# 06 OS-Aware Command Executor Task Pack

This task pack patches Ptolemy's generic command execution path so `workerd` can run commands on Windows and Linux/macOS.

## Why this exists
The current executor may assume:

```go
bash -lc <command>
```

That is correct on Linux/macOS but not reliable on Windows. The patch adds an OS-aware helper:

- Windows: PowerShell
- Linux/macOS: Bash

## Run Order
1. `inbox/01-discover-current-executor-shell-path.md`
2. `inbox/02-add-os-aware-shell-command-helper.md`
3. `inbox/03-replace-hardcoded-bash-execution.md`
4. `inbox/04-add-tests-for-os-aware-command-construction.md`
5. `inbox/05-rebuild-and-smoke-test-workerd.md`

## Recommended Ptolemy Run

```bash
curl -s -X POST http://localhost:8080/agent/run \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "YOUR_SESSION_ID",
    "task_file": "docs/tasks/packs/06-os-aware-command-executor/TASK_PLAN.md"
  }'
```

Or run inbox files one by one if your current agent performs better with smaller scoped tasks.
