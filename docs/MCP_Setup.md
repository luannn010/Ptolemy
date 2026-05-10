# Ptolemy MCP Setup

This guide shows how to:

1. build `ptolemy-mcp`
2. start the worker daemon it depends on
3. register Ptolemy as an MCP server in Codex
4. reuse the same server in other MCP clients that support stdio commands

## What `ptolemy-mcp` needs

`ptolemy-mcp` is a stdio MCP server. It talks to `workerd` over HTTP.

Before you add it to Codex or another MCP client, make sure:

- the repo is checked out locally
- Go is installed
- `workerd` is running
- the worker health check returns `ok`

## 1. Build the MCP server

From the repo root:

```bash
make build-mcp
```

That produces:

```text
bin/ptolemy-mcp
```

You can also run it without building first:

```bash
go run ./cmd/ptolemy-mcp
```

For Codex and most MCP clients, the built binary is the cleaner option.

If `make build-mcp` fails with `go: No such file or directory`, Go may be installed but missing from your shell `PATH`. In that case, confirm `go version` works first, or use the full Go binary path for one-off builds.

## 2. Start `workerd`

In a separate terminal, from the same repo:

```bash
go run ./cmd/workerd
```

By default, the worker listens on:

```text
http://127.0.0.1:8080
```

Verify it:

```bash
curl -s http://127.0.0.1:8080/health | jq
```

Expected result:

```json
{
  "status": "ok",
  "service": "workerd"
}
```

If your worker runs on a different URL, set `PTOLEMY_WORKER_URL` when launching `ptolemy-mcp`.

Example:

```bash
PTOLEMY_WORKER_URL=http://127.0.0.1:8080 ./bin/ptolemy-mcp
```

## 3. Optional local smoke test

Before wiring it into Codex, you can confirm the MCP binary starts:

```bash
PTOLEMY_WORKER_URL=http://127.0.0.1:8080 ./bin/ptolemy-mcp
```

It should wait quietly on stdin/stdout. That is normal for a stdio MCP server.

Stop it with `Ctrl+C`.

## 4. Add Ptolemy to Codex

Codex supports stdio MCP servers through:

```bash
codex mcp add <name> -- <command>...
```

Use the variant that matches where Codex runs.

### Option A: Codex running in the same Linux/macOS environment as the repo

From any shell:

```bash
codex mcp add ptolemy --env PTOLEMY_WORKER_URL=http://127.0.0.1:8080 -- /absolute/path/to/ptolemy/bin/ptolemy-mcp
```

Example:

```bash
codex mcp add ptolemy --env PTOLEMY_WORKER_URL=http://127.0.0.1:8080 -- /home/luannn010/projects/ptolemy/bin/ptolemy-mcp
```

### Option B: Codex on Windows, Ptolemy inside WSL

If Codex is launching commands from Windows and your repo lives in WSL, use `wsl.exe`:

```powershell
codex mcp add ptolemy -- wsl.exe -d Ubuntu -- bash -lc "cd /home/luannn010/projects/ptolemy && PTOLEMY_WORKER_URL=http://127.0.0.1:8080 ./bin/ptolemy-mcp"
```

This is usually the best fit for a Windows Codex app + Ubuntu WSL repo setup.

## 5. Verify the Codex registration

List configured MCP servers:

```bash
codex mcp list
```

Inspect the saved Ptolemy entry:

```bash
codex mcp get ptolemy
```

After that, restart Codex if the server does not appear immediately in a running session.

## 6. Add Ptolemy to other MCP clients

Any MCP client that supports stdio command servers can launch Ptolemy with the same binary.

Use this shape:

```json
{
  "mcpServers": {
    "ptolemy": {
      "command": "/absolute/path/to/ptolemy/bin/ptolemy-mcp",
      "env": {
        "PTOLEMY_WORKER_URL": "http://127.0.0.1:8080"
      }
    }
  }
}
```

For a Windows client launching into WSL, use:

```json
{
  "mcpServers": {
    "ptolemy": {
      "command": "wsl.exe",
      "args": [
        "-d",
        "Ubuntu",
        "--",
        "bash",
        "-lc",
        "cd /home/luannn010/projects/ptolemy && PTOLEMY_WORKER_URL=http://127.0.0.1:8080 ./bin/ptolemy-mcp"
      ]
    }
  }
}
```

Check your client's own docs for the exact config file location and field names, but the launch command above is the important part.

## Exposed MCP tool groups

`ptolemy-mcp` currently exposes tool groups for:

- sessions
- command execution
- file read/write/search/patch
- navigator and KB operations
- git operations
- worktree operations

Examples include:

- `ptolemy.create_session`
- `ptolemy.execute`
- `ptolemy.read_file`
- `ptolemy.search_codebase`
- `ptolemy.index_workspace`
- `ptolemy.kb_build`
- `ptolemy.git_status`
- `ptolemy.git_commit`
- `ptolemy.create_worktree`

## Troubleshooting

If Codex can start the MCP server but tool calls fail:

1. confirm `workerd` is still running
2. re-check `curl -s http://127.0.0.1:8080/health | jq`
3. confirm `PTOLEMY_WORKER_URL` matches the real worker URL
4. run the MCP command manually once to catch startup errors

If you get EOF or dropped execution responses from the worker, follow:

- `docs/workflows/recovery/eof-worker-drop.md`

## Related docs

- `docs/Setup.md`
- `docs/CLI.md`
- `docs/Worker_API.md`
- `WORKFLOWS.md`
