# Health Check Workflow

Verify the worker process is reachable.

```text
Client
  -> GET /health
  -> Worker responds with status/service/timestamp
```

Example:

```bash
curl -s http://localhost:8080/health
```

Status: working.
