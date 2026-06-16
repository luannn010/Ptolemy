# Brain Controller API

The brain controller lets `workerd` manage a local llama.cpp ("brain") process —
list models, load one with a full caller-supplied config, hibernate to free VRAM,
resume, and stop. It is a **loopback-only HTTP control plane**, and every action
goes through Ptolemy's policy harness (audited to `policy_decisions`).

- **Base URL:** `http://127.0.0.1:<BRAIN_CONTROL_PORT>` (default `http://127.0.0.1:8089`)
- **Binds:** loopback only (`127.0.0.1`). A request from any non-loopback address gets `403 {"error":"loopback only"}`. This is deliberate — the API can stop GPU processes.
- **Enabled when:** `BRAIN_CONTROL_ENABLED=true` (off by default). When disabled, none of these routes exist.
- **Co-location:** assumes `workerd` runs on the same host as the brain.

> The brain's *inference* endpoint (what you send prompts to) is the llama.cpp
> server itself (`BRAIN_BASE_URL`, OpenAI-compatible `/v1/...`). This control
> plane only manages that process's lifecycle. The RAG `/chat` data plane is a
> separate listener — see [the RAG note in Architecture.md](Architecture.md).

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `BRAIN_CONTROL_ENABLED` | `false` | Master switch for the whole controller + this API. |
| `BRAIN_CONTROL_PORT` | `8089` | Loopback port this API listens on. |
| `BRAIN_MODELS_DIR` | _(empty)_ | Directory scanned by `GET /brain/models` for `*.gguf`. |
| `BRAIN_LLAMA_BIN` | _(empty)_ | Default `llama-server` binary used when a load omits `binary`. |
| `BRAIN_AUTO_WAKE` | `false` | When true, `POST /chat` resumes the loaded model just-in-time before answering. |
| `BRAIN_IDLE_TTL` | `5m` | After this much inactivity, the brain auto-hibernates (frees VRAM, keeps the spec). |
| `BRAIN_BASE_URL` | `http://127.0.0.1:8088` | The brain's own host:port; its port is the default bind for a loaded spec, and `GET /v1/models` there is the readiness probe. |

## Core concepts

### The launch spec
A load is described by a free-form **spec** (there is no preset registry):

| Field | Type | Required | Notes |
|---|---|---|---|
| `gguf` | string | **yes** | Path to the model file (`-m`). |
| `binary` | string | no | `llama-server` path. Defaults to `BRAIN_LLAMA_BIN`. |
| `host` | string | no | `--host`. Defaults to the brain bind host (`0.0.0.0`). |
| `port` | string | no | `--port`. Defaults to the `BRAIN_BASE_URL` port. |
| `args` | string[] | no | Any extra llama.cpp flags, e.g. `["--ctx-size","32768","-ngl","999"]`. |

The Manager composes: `binary -m gguf --host <host> --port <port> <args...>`.

### Lifecycle
`load` sets the active spec → process runs. `hibernate` stops the process but
**keeps** the spec. `resume` (and `/chat` auto-wake) relaunch the **same** spec.
`stop` stops the process **and forgets** the spec. Only `load` introduces a new
spec; everything else acts on the stored one.

### Policy posture (which calls pause for approval)
| Action | Effect |
|---|---|
| `models`, `status`, `resume`, `hibernate`, auto-wake | **allow** — runs immediately (still audited) |
| `load`, `stop` | **ask / out-of-band** — pauses for human approval (see below) |

`load` is gated because it can launch an arbitrary binary. The **entire resolved
argv** is placed in the policy intent, so the deny rules (`rm -rf`, `.env`,
`git push --force`, …) reject a destructive token in any spec field, and the
approval token is bound to that exact spec.

## The approval flow (for `load` and `stop`)

1. Call the endpoint normally. If the action is gated, you get **`202`**:
   ```json
   { "status": "needs_confirmation", "channel": "oob", "pending_id": "<hash>", "reason": "custom brain launch requires confirmation" }
   ```
2. A human approves out-of-band on the **approve listener** (loopback, `APPROVE_PORT`, default `8081`):
   ```
   POST http://127.0.0.1:8081/approve/<pending_id>   →  200 {"status":"approved"}
   ```
3. **Retry the same call** with `confirm_token` set to the `pending_id`:
   ```json
   { "gguf": "...", "args": [...], "confirm_token": "<pending_id>" }
   ```
   Now it executes and returns `200 {"status":"ok"}`.

`confirm_token == pending_id == the intent hash`. Approving one spec cannot
authorize a different one (a changed field → different hash → fresh approval).

## Endpoints

### `GET /brain/models`  — list available models *(allow)*
Scans `BRAIN_MODELS_DIR` (recursively) for `*.gguf`.

**200**
```json
{ "models": [ { "name": "Qwen3.5-9B-Q4_K_M.gguf", "path": "/home/you/models/qwen3.5-9b/Qwen3.5-9B-Q4_K_M.gguf", "size": 5963776000 } ] }
```
Missing/empty `BRAIN_MODELS_DIR` → `{"models":[]}`.

### `GET /brain/status`  — current state *(allow)*
**200**
```json
{ "running": true, "hibernated": false, "binary": "/usr/local/bin/llama-server", "gguf": "/models/q.gguf", "host": "0.0.0.0", "port": "8088", "args": ["--ctx-size","32768"], "last_use": "2026-06-17T11:24:05Z" }
```
- `running` — process is up. `hibernated` — a spec is stored but not running.
- Spec fields (`binary`/`gguf`/`host`/`port`/`args`) are omitted when nothing has been loaded yet.

### `POST /brain/load`  — load a model *(ask/OOB)*
Body: the spec fields above (+ optional `confirm_token`).

```bash
curl -s -X POST http://127.0.0.1:8089/brain/load \
  -H 'Content-Type: application/json' \
  -d '{"gguf":"/models/qwen3.5-9b/Qwen3.5-9B-Q4_K_M.gguf","args":["--ctx-size","32768","-ngl","999","--batch-size","512","--threads","8"]}'
```
- Missing `gguf` → **400** `{"error":"gguf is required"}`.
- First call → **202** needs_confirmation. After approve, retry with `confirm_token` → **200** `{"status":"ok"}`.
- Loading a *different* spec than the running one swaps it (also `ask`).

### `POST /brain/resume`  — relaunch the stored spec *(allow)*
Body: none (or `{}`).
- **200** `{"status":"ok"}` — relaunched (or already running; no-op).
- **409** `{"error":"no model loaded"}` — nothing has been loaded yet (cold start). Call `/brain/load` first.

### `POST /brain/hibernate`  — free VRAM, keep the spec *(allow)*
Body: none. Stops the process; the spec is retained so `resume`/auto-wake bring back the same model.
- **200** `{"status":"ok"}`.

### `POST /brain/stop`  — stop and forget *(ask/OOB)*
Body: optional `{"confirm_token":"..."}`. Stops the process **and clears** the stored spec (next `resume` → 409 until you load again).
- First call → **202** needs_confirmation; after approve, retry with `confirm_token` → **200** `{"status":"ok"}`.

## Status codes

| Code | Meaning | Body |
|---|---|---|
| `200` | Success | `{"status":"ok"}`, or the `status`/`models` payload |
| `202` | Needs approval (`load`/`stop`) | `{"status":"needs_confirmation","channel":"oob","pending_id":"…","reason":"…"}` |
| `400` | Bad request (e.g. missing `gguf`, invalid JSON) | `{"error":"…"}` |
| `403` | Denied by policy, **or** non-loopback caller | `{"error":"…","rule_id":"…"}` / `{"error":"loopback only"}` |
| `409` | `resume` with no model loaded (cold start) | `{"error":"no model loaded"}` |
| `502` | Process/exec failure (launch failed, readiness timeout) | `{"error":"…"}` |

## Auto-wake on `/chat` (related)
When `BRAIN_AUTO_WAKE=true`, the RAG listener's `POST /chat` calls the brain's
`resume` before answering, so a request after idle reloads the model just-in-time
(the first call pays model-load latency). Cold start with nothing ever loaded →
`/chat` returns `502`. This uses the same allow-gated wake path; it never loads a
*new* spec.

## Worked example (full cycle)

```bash
B=http://127.0.0.1:8089            # brain control plane
A=http://127.0.0.1:8081            # approve listener

# 1. see what's available
curl -s $B/brain/models

# 2. load (gated) -> grab the pending id
P=$(curl -s -X POST $B/brain/load -H 'Content-Type: application/json' \
     -d '{"gguf":"/models/qwen3.5-9b/Qwen3.5-9B-Q4_K_M.gguf","args":["--ctx-size","32768","-ngl","999"]}' \
     | jq -r .pending_id)

# 3. approve out-of-band, then retry with the token
curl -s -X POST $A/approve/$P
curl -s -X POST $B/brain/load -H 'Content-Type: application/json' \
     -d "{\"gguf\":\"/models/qwen3.5-9b/Qwen3.5-9B-Q4_K_M.gguf\",\"args\":[\"--ctx-size\",\"32768\",\"-ngl\",\"999\"],\"confirm_token\":\"$P\"}"

# 4. it's up — inference now goes to the brain itself (BRAIN_BASE_URL), e.g. :8088/v1/chat/completions
curl -s $B/brain/status

# 5. free VRAM, then bring the SAME model back
curl -s -X POST $B/brain/hibernate
curl -s -X POST $B/brain/resume
```

## Reusing from another project

The control plane is **loopback-only by design** — it can kill GPU processes, so
it is never exposed off-host. To drive it from another machine, do **not** open
`8089` to the network; instead tunnel loopback over SSH:

```bash
ssh -N -L 8089:127.0.0.1:8089 -L 8081:127.0.0.1:8081 user@brain-host
# now 127.0.0.1:8089 on your machine reaches the brain host's control plane
```

If a caller only needs to *ask questions* (not manage the model), use the RAG
data plane instead: `POST <host>:8090/chat` (that listener binds all interfaces;
see its security note in the README). With `BRAIN_AUTO_WAKE=true`, `/chat` handles
waking the model for you — most consumers never touch this control plane at all.

## Notes

- **Hibernate is stop + relaunch.** llama.cpp has no in-process "offload and hold"; `hibernate` stops the process and `resume` relaunches the same argv (OS file cache makes the reload faster, but the model reloads).
- **All actions are audited** to `policy_decisions` under a reserved `brain-system` session, whether allowed, asked, or denied.
- **Readiness:** a load returns only after the brain answers `GET /v1/models` on the spec's host:port (or fails with `502` on timeout).
