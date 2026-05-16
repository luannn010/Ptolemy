# Health Check Workflow

Verify worker health plus MCP/runtime readiness.

```text
Client
  -> GET /health
  -> Worker responds with status/service/timestamp/checks
```

Example:

```bash
curl -s http://localhost:8080/health
```

Example response fields:

- `status`: `ok` or `degraded`
- `service`: `workerd`
- `checks.mcp`: MCP reachability when `MCP_BASE_URL` is configured
- `checks.runtime.commands.go|npm|python`: runtime command availability

Troubleshooting:

- MCP failed: confirm `MCP_BASE_URL` and that MCP server is running and reachable.
- `go` missing: install Go and ensure it is on PATH for the worker process.
- `npm` missing: install Node.js/npm and ensure PATH visibility.
- `python` missing: install Python and ensure PATH visibility.
