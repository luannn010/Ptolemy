# Brain controller — design spec

**Date:** 2026-06-14
**Branch:** `ptolemy/brain-controller` (off the brain commits; RAG listener split to its own PR #48)
**Status:** approved in brainstorming; pending written-spec review → writing-plans → TDD

## Problem

workerd already has a guarded brain skill (start/stop/switch a llama.cpp process,
auto-wake on `/chat`, idle-TTL unload) built around a **named-preset registry**
(`internal/brain/models.go`: `Model{Name,Binary,GGUF,Args}`). The user wants a
fuller controller: drive llama.cpp the way they would from the CLI — **list
available models, load any of them with a full caller-supplied config, and
hibernate → resume** to free/reclaim VRAM — not be limited to pre-baked presets.

Example config the controller must be able to express verbatim:

```
~/projects/llama.cpp/build/bin/llama-server \
  -m ~/models/qwen3.5-9b/Qwen3.5-9B-Q4_K_M.gguf \
  --host 0.0.0.0 --port 9000 \
  --ctx-size 32768 -ngl 999 -np 1 \
  --batch-size 512 --ubatch-size 512 --threads 8
```

This is a **behavior change** to the existing (unpushed) brain skill: it removes
the registry and reshapes the lifecycle verbs and policy intents. Because spawning
a process is a side effect, everything stays behind `policy.GuardedBrain` (the
harness), per CLAUDE.md.

## Decisions (locked in brainstorming)

1. **Config model: fully free-form, binary included.** The caller supplies the
   whole launch spec (binary, gguf, host, port, flags) at load time. Consequence
   (accepted): a custom-config load can launch an arbitrary executable, so it is
   **`ask`/OOB** (human approval on the loopback approve port) — never auto-allow.
2. **Discovery: disk scan only.** `GET /brain/models` enumerates `*.gguf` under a
   configured `BRAIN_MODELS_DIR`. No named presets.
3. **Hibernate → resume: auto-resume last, no re-approval.** Once a spec is
   approved and loaded, idle/manual hibernate frees VRAM but remembers the spec;
   the next `/chat` (or `POST /brain/resume`) relaunches that exact spec with no
   new approval. Loading a *different* spec asks again. Cold start (nothing ever
   loaded) → `/chat` returns 502 "no model loaded — load one first."
4. **Approach: extend `internal/brain` in place** (vs. a new wrapper layer or
   delegating to llama-swap). Reuses the single-process mutex owner, the
   guard/audit path, and the idle loop.

## Security invariant (load-bearing)

> A custom spec can enter the Manager **only** through the `load` path, which is
> `ask`/OOB. Every other verb (`resume` / `wake` / `hibernate` / `status` /
> `models`) carries **no spec payload** — it acts only on the already-approved
> stored spec — so it is safe to auto-`allow`. (`stop` also carries no spec; it
> is kept `ask` as a deliberate manual-teardown gate, not because it is unsafe.)
> A caller can never smuggle a new binary in through `resume`.

Reinforced for free by the policy engine: `Authorize` matches on
`program + args + targets` (`engine.go:28`), so the **entire argv** of a load
(binary + gguf + every flag) is matched against all rules, including the `deny`
rules. `rm -rf`, `.env`, `git push --force` in any spec field are therefore
auto-denied — a loopback caller cannot pass a destructive token as a "flag."

## Data model

```go
// Spec is one free-form llama.cpp launch. Replaces the named-preset Model.
type Spec struct {
    Binary string   // llama-server path (defaults to BRAIN_LLAMA_BIN if empty)
    GGUF   string   // -m <file>
    Host   string   // --host (defaults to configured brain bind host)
    Port   string   // --port (defaults to configured brain bind port)
    Args   []string // remaining flags: --ctx-size, -ngl, --batch-size, ...
}
```

Manager state:

- `spec Spec` — the spec to (re)launch. **Set only by an approved Load. Persists
  across Hibernate. Cleared only by Stop.**
- `handle Handle` — the running process (nil when hibernated/stopped).
- `lastUse time.Time` — drives the idle-TTL loop.

`argv(spec)` = `[binary, "-m", gguf, "--host", host, "--port", port] + args`.

## Lifecycle verbs → loopback endpoints → policy

The control plane stays **loopback-only** (`127.0.0.1:BRAIN_CONTROL_PORT`); it can
kill GPU processes.

| Endpoint | Spec in body? | Intent (`program=brain`) | Effect |
|---|---|---|---|
| `GET /brain/models` | no | `brain models` | **allow** (read) |
| `GET /brain/status` | no | `brain status` | **allow** (read) |
| `POST /brain/load` `{gguf, binary?, host?, port?, args?[], confirm_token?}` | **yes** | `brain load <argv…>` | **ask/OOB** |
| `POST /brain/resume` `{confirm_token?}` | no (stored spec) | `brain resume` | **allow** |
| `POST /brain/hibernate` | no | `brain hibernate` | **allow** |
| `POST /brain/stop` `{confirm_token?}` | no | `brain stop` | **ask/OOB** |

- `/chat` auto-wake = `EnsureAwake` (intent `brain wake`, allow) → resumes the
  stored spec; no stored spec → 502 "no model loaded" (an upstream-unavailable
  class error for the RAG caller).
- Explicit `POST /brain/resume` with no stored spec → **409 Conflict** "no model
  loaded — load one first" (valid request, wrong state).
- `load` with an empty `binary` defaults to `BRAIN_LLAMA_BIN`.
- "Switch" is removed — loading a different spec *is* the switch (and is `ask`,
  since the differing argv → a different intent hash → a fresh approval).
- The idle-TTL loop calls **hibernate** (keeps the spec) instead of the old
  `unload` (which cleared it), so the next `/chat` resumes the *same* model.

`ask` flow (unchanged): `202 needs_confirmation {pending_id}` → operator
`POST 127.0.0.1:APPROVE_PORT/approve/{pending_id}` → retry with
`confirm_token = pending_id` (the intent hash).

## Policy rules (allow/ask only — denies + self-protection untouched)

Add to `DefaultRuleset()` (`rules.go`) **and** the host-local `.ptolemy/policy.json`:

- `allow-brain-models` — contains `brain models`
- `allow-brain-resume` — contains `brain resume`
- `allow-brain-hibernate` — contains `brain hibernate`
- `ask-brain-load` — contains `brain load`, channel oob

Keep: `allow-brain-wake`, `allow-brain-status`, `ask-brain-stop`.
Remove: `ask-brain-switch`, `allow-brain-autounload` (folded into hibernate/load).

## Config knobs (config.go)

- `BRAIN_MODELS_DIR` — root scanned by `GET /brain/models` (read-only, scoped).
- `BRAIN_LLAMA_BIN` — default binary for loads that omit it.
- Spec `host`/`port` default to the configured brain bind (derived from
  `BRAIN_BASE_URL`).
- **Removed:** `BRAIN_MODELS` (registry path) — no presets anymore.
- Unchanged: `BRAIN_CONTROL_ENABLED`, `BRAIN_AUTO_WAKE`, `BRAIN_IDLE_TTL`,
  `BRAIN_CONTROL_PORT`.

⚠️ **Port coupling:** RAG/`/chat`, the embedder client, and `/health` talk to
`BRAIN_BASE_URL`. If a load overrides `port` to something else, the readiness
probe follows the spec but those consumers do not. For the brain that serves
`/chat`, keep the default port.

## Probe change

Today the readiness probe (`NewHTTPProbe`) is bound to a fixed `BrainBaseURL`.
It must poll the **active spec's** `host:port` (`GET /v1/models`) so a custom-port
load is detected correctly. The Manager builds the probe target from the spec
(probe becomes per-load, or `Ready(ctx, baseURL)` takes the target).

## Files

| File | Change |
|---|---|
| `internal/brain/spec.go` (new) + test | `Spec` type, `argv(spec)`, default-fill (binary/host/port), validation. |
| `internal/brain/discovery.go` (new) + test | Scan `BRAIN_MODELS_DIR` for `*.gguf`; ignore non-gguf; empty/missing dir → empty list, no error. |
| `internal/brain/manager.go` (rewrite) + test | `Load(spec)`, `Resume`, `EnsureAwake`, `Hibernate`, `Stop`, `Status`, `ListModels`; `spec` persists across hibernate, cleared by stop; probe targets spec host:port. |
| `internal/brain/models.go` (remove) + test | Delete the named-preset registry. |
| `internal/policy/guard_brain.go` (rewrite) + test | `RawBrain`: `Load/Resume/EnsureAwake/Hibernate/Stop/Status/ListModels`. Guard methods gate on the intents above. Remove `Wake(model)`/`Switch`/`Unload`. |
| `internal/policy/rules.go` (edit) + test | The rule changes above. |
| `internal/httpapi/brain.go` (rewrite) + test | Routes above; loopback-only; `load` 202→approve→retry; `resume` with no stored spec → 409; stop ask. |
| `internal/config/config.go` (edit) + test | `BRAIN_MODELS_DIR`, `BRAIN_LLAMA_BIN`; drop `BRAIN_MODELS`. |
| `cmd/workerd/brain.go` (edit) + test | `buildBrainDeps`: drop registry load; require/derive `BRAIN_LLAMA_BIN` + `BRAIN_MODELS_DIR`; idle loop → `Hibernate`; `brain-system` session unchanged. |
| `cmd/workerd/main.go` (edit) | Wire the new routes (still gated by `BRAIN_CONTROL_ENABLED`). |
| `brain-models.example.json` (remove) | Replaced by a `BRAIN_MODELS_DIR` note. |
| `docs/Architecture.md`, `.env.example` (edit) | Update the brain note + env block. |

## TDD order (tests first each step)

1. **config** — `BRAIN_MODELS_DIR`/`BRAIN_LLAMA_BIN` parse + default; `BRAIN_MODELS` gone.
2. **brain.Spec** — `argv` composition; default-fill of binary/host/port; validation (empty gguf errors).
3. **brain.discovery** — finds `*.gguf`, ignores others, missing/empty dir → empty.
4. **brain.Manager** (fake Launcher + stub Probe): Load launches + waits ready;
   spec persists across Hibernate; Resume relaunches stored spec; Resume with no
   spec errors; Stop clears spec; EnsureAwake no-ops when running, resumes when
   hibernated, errors cold; probe targets spec host:port; idle loop → Hibernate.
5. **policy.GuardedBrain** (stub RawBrain + real Engine/Approvals/in-mem sqlite):
   `load` → ask + parked + raw NOT called; valid confirm_token → raw called +
   `confirmed=1`; `resume`/`hibernate`/`status`/`models` → allow + audit row;
   `stop` → ask; a deny token in a spec field → ErrDenied.
6. **httpapi.brain** (stub GuardedBrain): loopback-only (non-loopback→403);
   models/status happy paths; load→202 needs_confirmation; stop→202; resume with
   no stored spec→409; method/route checks.
7. **cmd/workerd.buildBrainDeps** — disabled without `BRAIN_CONTROL_ENABLED`;
   enabled wires dir+default-bin and ensures the `brain-system` session.
8. **main.go** wiring — `go build` + `go vet` (main untested by convention).
9. **docs + policy.json + .env.example.**

Commit per phase, explicit staging. No push until the user states merge intent.

## Verification

- **Now (TDD):** `go test -p 1 ./...` green via fakes (fake launcher, stub probe,
  stub guard, in-mem sqlite) — no real process/DB/brain. `go build`/`go vet` clean.
  Flag-off default proven: `BRAIN_CONTROL_ENABLED` unset → `buildBrainDeps` ok=false,
  workerd serves unchanged.
- **Later (live, co-located on Linux/WSL):** `GET /brain/models` lists ggufs under
  `BRAIN_MODELS_DIR`; `POST /brain/load {gguf,...}` → 202 → approve → loads with the
  given config; `POST /brain/hibernate` frees VRAM; `/chat` auto-resumes the same
  spec; `policy_decisions` shows `ask` rows for load/stop and `allow` rows for the rest.

## Caveats / out of scope

- **llama.cpp has no in-process VRAM "offload-and-hold."** Hibernate = graceful
  stop; resume = relaunch the same argv. OS file cache speeds the reload, but the
  model reloads. This is the realistic interpretation of "temporarily offload."
- Live verification needs workerd **co-located** with the brain (Linux/WSL); ships
  TDD-complete, flag-off by default.
- Not a replacement for llama-swap — this guarded controller could *call* it later
  if the user prefers proxy-based idle-TTL; separate option.
- No new tables; no `deny`/self-protection changes; control plane loopback-only.
