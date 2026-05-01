# Task Script: MCP Wrapper Implementation

Implementation checklist:

1. use Python stdlib only
2. support line-delimited JSON-RPC over STDIN and STDOUT
3. implement `initialize`, `tools/list`, and `tools/call`
4. add tools:
   - `ptolemy_health`
   - `ptolemy_create_session`
   - `ptolemy_execute`
   - `ptolemy_run_task_file`
5. use `PTOLEMY_BASE_URL`, `PTOLEMY_DEFAULT_SESSION_ID`, and `PTOLEMY_AUTH_TOKEN`
6. add safe default HTTP timeouts
7. do not print secrets
8. return a clean fallback when `/agent/run` is unavailable
9. add `--self-test`
