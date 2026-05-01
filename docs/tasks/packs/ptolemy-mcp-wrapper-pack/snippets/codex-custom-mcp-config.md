# Codex Custom MCP Configuration

Use these exact fields in the Codex Custom MCP UI:

- Name: `Ptolemy`
- Type: `STDIO`
- Command: `python`
- Arguments: `scripts/mcp/ptolemy_mcp.py`
- Working directory: repo root
- Environment variable: `PTOLEMY_BASE_URL=http://<tailscale-ip>:8080`

Optional environment variables:

- `PTOLEMY_DEFAULT_SESSION_ID`
- `PTOLEMY_AUTH_TOKEN`
