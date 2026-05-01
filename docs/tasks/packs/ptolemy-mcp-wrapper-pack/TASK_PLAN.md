# Task Plan: Ptolemy STDIO MCP Wrapper

## Goal

Add a small STDIO MCP wrapper that runs on a remote or local Codex machine and forwards tool calls to an existing Ptolemy worker over HTTP or Tailscale.

## Execution Strategy

Use sequential-first execution.

The work is split into four tasks:

1. discover the live worker API shape already present in the repo
2. add the Python MCP wrapper bridge
3. add environment-driven configuration and operator docs
4. run smoke tests and record the result

## Global Constraints

- Do not replace `workerd`.
- Keep the wrapper as a thin bridge to the existing HTTP API.
- Do not expose secrets in examples, logs, or output.
- Prefer Python stdlib for the wrapper unless the repo requires a stronger existing MCP pattern.
- Stop if the implementation needs broad unrelated refactors.

## Execution Order

### Phase 1 - Discover and implement
1. `01-discover-worker-api.md`
2. `02-add-mcp-wrapper.md`

### Phase 2 - Configure and validate
3. `03-add-config-and-docs.md`
4. `04-test-mcp-wrapper.md`

## Required Behavior

The wrapper must expose these MCP tools:

- `ptolemy_health`
- `ptolemy_create_session`
- `ptolemy_execute`
- `ptolemy_run_task_file`

The wrapper must use these environment variables:

- `PTOLEMY_BASE_URL` with default `http://127.0.0.1:8080`
- `PTOLEMY_DEFAULT_SESSION_ID` optional
- `PTOLEMY_AUTH_TOKEN` optional future support

The wrapper must:

- use safe request timeouts
- return clean JSON or text results to the MCP client
- avoid logging or echoing secrets
- report clearly when `/agent/run` is not available on the worker

## Global Validation

```bash
python scripts/mcp/ptolemy_mcp.py --self-test
curl -s http://127.0.0.1:8080/health
/usr/local/go/bin/go test ./...
```

## Completion Policy

This pack is complete only when:

- the wrapper script exists and can boot over STDIO
- the docs show the exact Codex Custom MCP configuration
- the smoke tests are recorded
- pack planning succeeds

## Failure Rule

If automated pack execution would push or create a PR without explicit approval, stop and record a fallback note instead of running the full pack.
