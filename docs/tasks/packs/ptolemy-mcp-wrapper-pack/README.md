# Ptolemy MCP Wrapper Pack

This pack adds a small STDIO MCP bridge that talks to an existing Ptolemy worker over HTTP.

## Goal

Keep `workerd` as the real runtime and expose a lightweight bridge for Codex or ChatGPT Custom MCP:

```text
Codex or ChatGPT MCP client
  -> scripts/mcp/ptolemy_mcp.py
  -> PTOLEMY_BASE_URL over HTTP or Tailscale
  -> existing workerd
```

## Pack Contents

```text
ptolemy-mcp-wrapper-pack/
|-- TASK_PLAN.md
|-- PACK_MANIFEST.yaml
|-- README.md
|-- inbox/
|   |-- 01-discover-worker-api.md
|   |-- 02-add-mcp-wrapper.md
|   |-- 03-add-config-and-docs.md
|   `-- 04-test-mcp-wrapper.md
|-- scripts/
|   `-- fallback-note.md
|-- snippets/
|   `-- codex-custom-mcp-config.md
`-- task-scripts/
    |-- 01-worker-api-discovery.md
    |-- 02-mcp-wrapper-implementation.md
    |-- 03-config-and-docs.md
    `-- 04-smoke-tests.md
```

## Codex Custom MCP

Use these values in the Codex Custom MCP UI:

- Name: `Ptolemy`
- Type: `STDIO`
- Command: `python`
- Arguments: `scripts/mcp/ptolemy_mcp.py`
- Working directory: repo root
- Environment variable: `PTOLEMY_BASE_URL=http://<tailscale-ip>:8080`

Optional environment variables:

- `PTOLEMY_DEFAULT_SESSION_ID`
- `PTOLEMY_AUTH_TOKEN`

## Run Strategy

Use `plan --pack` safely for validation preview.

Do not use the current full `run --pack` flow without explicit approval because the current pack runtime prepares branches, pushes, and drafts PRs at the end.
