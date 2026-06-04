# Deploying Ptolemy: systemd + Codex (MCP)

This guide covers running Ptolemy on a Linux host with **systemd** for the
long-running pieces and **Codex** as the MCP client.

## The key distinction

| Component | What it is | How it runs |
|---|---|---|
| `workerd` | Long-running HTTP daemon (API on `:HTTP_PORT`, default 8080, plus a loopback approval server) | **systemd service** |
| `ptolemy-mcp` | MCP server, **stdio only** (`server.Run(os.Stdin, os.Stdout)` — no network listener) | **Spawned by Codex** per session — NOT a systemd service |

You never `systemctl start ptolemy-mcp`. systemd keeps `workerd` and the
dependencies (Postgres, brain LLM, embeddings) up; Codex launches `ptolemy-mcp`
on demand and feeds it env.

> The memory tools (`ptolemy_memory_recall` / `_capture` / `_consolidate`) run
> **in-process** inside `ptolemy-mcp` and talk straight to Postgres — they do
> **not** require `workerd`. `workerd` is only needed for the session/executor
> tools. So to test capture/recall you only need Postgres + brain + embeddings.

## 1. Build (on the Linux host)

```bash
cd /opt/ptolemy        # your checkout
make build             # -> bin/workerd  bin/ptolemy-mcp  bin/ptolemy  bin/ptolemy-memory
```

## 2. Configure env

```bash
sudo install -d /etc/ptolemy
sudo cp .env.example /etc/ptolemy/ptolemy.env
sudoedit /etc/ptolemy/ptolemy.env
```

Essentials (rest can stay default — see `.env.example`):

```ini
DATABASE_URL=postgres://ptolemy:ptolemy@localhost:5432/ptolemy?sslmode=disable
BRAIN_BASE_URL=http://127.0.0.1:8088
BRAIN_MODEL=your-chat-model
EMBEDDING_BASE_URL=http://127.0.0.1:8000
EMBEDDING_MODEL=bge-large-en-v1.5
EMBEDDING_DIM=1024
AGENT_LOOP_ENABLED=true     # recall(generate/trace) routes through agentic RAG
AGENT_MAX_STEPS=5
```

## 3. systemd service for `workerd`

Use the template at [`deploy/ptolemy-workerd.service`](../deploy/ptolemy-workerd.service):

```bash
sudo cp deploy/ptolemy-workerd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ptolemy-workerd
systemctl status ptolemy-workerd
curl -s localhost:8080/health | jq    # deep readiness probe
```

Make Postgres / brain / embeddings their own services (or ensure they're
running) and add them to the unit's `After=` / `Wants=` so workerd starts after
they're up.

## 4. Point Codex at `ptolemy-mcp`

Copy the `[mcp_servers.ptolemy]` block from
[`deploy/codex.config.toml.example`](../deploy/codex.config.toml.example) into
`~/.codex/config.toml`. The binary must live on the **same machine Codex runs
on**; the `*_BASE_URL` / `DATABASE_URL` values must be reachable from there.

Restart Codex so it picks up the new server, then confirm it's registered
(e.g. `codex mcp list`).

## 5. Test

### A. Standalone smoke (no Codex)

Pipe JSON-RPC straight into the binary — the fastest way to confirm capture +
recall + the reasoning trace:

```bash
cd /opt/ptolemy && set -a; . /etc/ptolemy/ptolemy.env; set +a
printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ptolemy_memory_capture","arguments":{"user_text":"BRAIN runs on port 8088","assistant_text":"noted"}}}' \
 '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ptolemy_memory_recall","arguments":{"query":"which port is BRAIN on","trace":true}}}' \
 | ./bin/ptolemy-mcp | jq
```

Expect: id 2 lists the tools (recall advertises a `trace` arg); id 3 returns
`{"captured":true}`; id 4 returns `{text, citations, mode:"agentic", steps:[...]}`
— the reasoning trace (queries issued, chunks with id/score/snippet, grounding).

### B. Through Codex

- `ptolemy_memory_capture { user_text: "...", assistant_text: "..." }` → stored.
- `ptolemy_memory_recall { query: "...", trace: true }` → the LLM runs and you
  get the `steps` trace (agentic because `AGENT_LOOP_ENABLED=true`; without the
  flag, recall still works but uses the legacy single-shot path).

## Notes

- **No DATABASE_URL** → memory tools are omitted; session/executor tools still
  work (and still need `workerd` + `WORKER_BASE_URL`).
- **Agentic vs legacy**: `AGENT_LOOP_ENABLED=true` makes `recall(generate|trace)`
  loop (plan → retrieve → reason → answer/give-up). Off = legacy single-shot.
- `ptolemy-mcp` autoloads a `.env` from its CWD via godotenv, but when Codex
  spawns it the CWD is not the repo — so pass config via the Codex `env` block.
