# Planner vs Executor Workflow

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
