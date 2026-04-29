# Invalid Multi-Action Recovery Workflow

Reject invalid model output before any action executes.

```text
Agent
  -> Validate raw model reply
  -> Reject multiple top-level JSON objects or arrays
  -> Store raw invalid output in actions/logs
  -> Return structured invalid_model_output result
  -> Optionally ask for split_into_task_batch next
  -> Queue valid create_task_batch children as pending actions
  -> Execute only one validated action per step
```

Recovery sequence:

1. Detect invalid output.
2. Stop execution.
3. Log raw response.
4. Return structured error.
5. Optionally split into task batch.
6. Queue tasks.
7. Execute one task at a time later.

Status: planned and enforced in `ptolemy-agent` parser/recovery flow.
