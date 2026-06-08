# Controller — Stage-2 Integration Lock & Serial Promotion

**Date:** 2026-06-07
**Status:** Approved (brainstorming) — pending implementation plan
**Slice:** 2 of N for the Ptolemy multi-agent orchestration layer
**Builds on:** `docs/superpowers/specs/2026-06-07-controller-worker-pool-design.md`

## Context

Slice 1 built the deterministic spine of the orchestration layer in
`internal/controller`: an event bus, a worker lifecycle state machine, and a
supervisor that spawns N workers into git worktrees and drives each through an
injected `Runner` up to `Stage1Passed` (parallel, bounded by `MaxWorkers`).

This slice implements the next step in the architecture's build order
(`docs/ptolemy-architecture.html` §01, §03, §07):

> ... integration lock + Stage 2 → base_updated propagation ...

After a worker's Stage-1 work goes green it must be promoted **serially** against
the one real environment: acquire a single integration lock (one worker at a
time, with crash reclaim), test against the real env (Stage 2), and on success
**merge to base**, emitting a `base_updated` event. The promotion is serial
because there is exactly one real environment; Stage-1 stays parallel.

"Keep the model out of the plumbing" still governs: the lock, the merge, and the
event dispatch are deterministic Go. Stage-2 execution is an injected interface
whose real (model/test-harness-backed) implementation lands later.

## Goals

- A serial **integration lock** with automatic crash reclaim, backed by a
  Postgres advisory lock in production and an in-memory fake in unit tests.
- Drive a `Stage1Passed` worker through `Integrating → Merged`: acquire lock →
  run Stage 2 → merge to base via a guarded merger → emit `base_updated`.
- Keep the controller core free of `pgx` and `git` concrete types — everything
  is an injected interface, exactly as slice 1 did with `WorktreeManager`.
- Backward compatibility: when the integration deps are not configured, the
  supervisor behaves exactly as in slice 1 (stops at `Stage1Passed`).
- Full test coverage, written test-first.

## Non-goals (deferred to later slices)

- `base_updated` **subscribers**: code propagation to in-flight siblings,
  mock-update sender, re-test trigger, the relevance filter. This slice only
  *emits* `base_updated`; nothing consumes it yet.
- The Stage-2-failure → "encode as a new red test, re-enter TDD" loop.
- The integration **entry ticket** ("current with base" re-sync before the lock).
  Within a single `Run`, integration is serial and base changes only via these
  merges, so cross-`Run` staleness is the propagation slice's concern.
- Code-conflict **resolution** policy (human resolves vs worker re-tasked) — an
  open decision. A merge conflict here is simply a `Failed` worker.
- Real-env **state reset** between serial runs — an open decision; out of scope.
- Docker/`docker compose -p` stacks, HTTP mocks, OpenAPI contracts.
- Wiring in the model / orchestrator.

## Governance: why the Postgres lock needs no `Guarded*` wrapper

CLAUDE.md restricts the side-effecting **guarded adapters** (`shellcmd`,
`terminal`, `fileops`, `gitops`, `worktree`, `workspace`, `inspect`) to be
reachable only via `Guarded*` wrappers, and grants `internal/memory` /
`navigator` a Postgres carve-out. A `pg_advisory_lock` is neither a guarded
adapter nor a workspace/shell/git mutation — it is ephemeral coordination.
`internal/health` already establishes the precedent: it pings Postgres via
`pgxpool` without a guard, and its Architecture.md note documents that read-only
/ non-workspace Postgres touches need no `Guarded*` wrapper. The integration
lock follows that precedent. The actual code change a promotion makes — the
**merge to base** — *does* go through `GuardedGit`, preserving the harness rule
for the git side effect.

## Architecture & boundaries

Package `internal/controller`, extended with new injected seams.

| Unit | File | Side effects | Guard? |
|------|------|--------------|--------|
| `IntegrationLock` interface | `lock.go` | none (interface) | n/a |
| Postgres lock implementation | `pglock.go` | Postgres advisory lock | no (health precedent) |
| `Stage2Runner`, `GitMerger` interfaces | `stage2.go` | none (interfaces) | n/a |
| Integration drive | `supervisor.go` (extended) | merge via injected `GitMerger` | **only via `*policy.GuardedGit`** |

The Postgres implementation lives in its own file (`pglock.go`) so the controller
core stays `pgx`-free for fast unit builds; only that file imports `pgx`.

## Components

### Integration lock (`lock.go`, `pglock.go`)

```go
// IntegrationLock serializes Stage-2 promotion: at most one holder at a time.
type IntegrationLock interface {
    // Acquire blocks until the lock is held or ctx is done. The returned release
    // func is idempotent and must be called (defer) to free the lock.
    Acquire(ctx context.Context) (release func(), err error)
}
```

**Postgres implementation** (`pglock.go`): acquires a dedicated connection from a
`*pgxpool.Pool`, runs `SELECT pg_advisory_lock($1)` with a fixed 64-bit key;
`release` runs `pg_advisory_unlock($1)` and returns the connection to the pool.
Because a session-level advisory lock is bound to its connection, a crashed
holder's connection drops and Postgres releases the lock automatically — this is
the crash-reclaim mechanism. A configurable max-hold deadline (lease) is applied
to the Stage-2 context so a live-but-stuck holder cannot pin the env forever.

```go
type PgLock struct { pool PgConnPool; key int64; lease time.Duration }
func NewPgLock(pool PgConnPool, key int64, lease time.Duration) *PgLock
```

`PgConnPool` is a tiny consumer interface (satisfied by `*pgxpool.Pool`) so the
lock can be unit-tested against a fake pool if desired; the primary unit-test
path uses an in-memory fake of `IntegrationLock` itself.

### Stage-2 runner and merger (`stage2.go`)

```go
// Stage2Runner tests a worker against the real environment. Outcome.Passed=false
// is a clean red; a non-nil error is an execution fault. (Outcome is reused from
// slice 1, runner.go.)
type Stage2Runner interface {
    RunStage2(ctx context.Context, w Worker) (Outcome, error)
}

// GitMerger performs the no-fast-forward merge of a worker's branch to base and
// reports the resulting base SHA. Satisfied by *policy.GuardedGit.
type GitMerger interface {
    MergeNoFF(ctx context.Context, sessionID, branch string, opts policy.CallOpts) (gitops.Result, error)
    CurrentCommitSHA(ctx context.Context, sessionID string, opts policy.CallOpts) (gitops.Result, error)
}
```

### Supervisor extension (`supervisor.go`)

`Deps` gains optional integration fields; `Config` gains the base branch:

```go
type Deps struct {
    Worktree WorktreeManager
    Runner   Runner
    Bus      *Bus
    Config   Config
    // Integration (slice 2) — all nil means slice-1 behavior (stop at Stage1Passed).
    Lock    IntegrationLock
    Stage2  Stage2Runner
    Merger  GitMerger
}

type Config struct {
    MaxWorkers int
    BaseBranch string // merge target; defaults to "main" when empty and integration is configured
}
```

`driveWorker` continues past `Stage1Passed` **iff** `Lock`, `Stage2`, and
`Merger` are all non-nil. A helper predicate `integrationConfigured()` reports
whether all three are set. **Partial** configuration (some but not all set) is a
wiring bug, so `New` panics on it — fail fast at startup rather than half-promote.
`New` keeps its slice-1 signature (`func New(Deps) *Supervisor`, no error return),
so existing callers are unaffected.

## Data flow (happy path, extends `driveWorker`)

1. Worker reaches `Stage1Passed` (slice-1 flow).
2. If integration configured: `release, err := Lock.Acquire(ctx)`; `defer release()`.
3. Transition `Stage1Passed → Integrating` (event).
4. `outcome, err := Stage2.RunStage2(ctx, snapshot)`.
5. On `Passed`: `Merger.MergeNoFF(ctx, sessionID, worker.Branch, opts)`; on success,
   read new SHA via `CurrentCommitSHA`; transition `Integrating → Merged` (event)
   and publish `Event{Type: EventBaseUpdated, WorkerID, Payload: sha}`.
6. Release lock (defer); next queued worker proceeds.

Stage-1 remains parallel (`MaxWorkers`). The lock makes only steps 2–6 serial, so
multiple workers can be in Stage-1 while exactly one is integrating.

## Error handling

- Stage-2 clean red (`Outcome{Passed:false}`) → `Integrating → Failed` with
  detail; lock released; siblings unaffected.
- Stage-2 runner error → `Failed` with the error detail.
- Merge error / conflict (`gitops.Result.Success == false` or non-nil error) →
  `Failed` with the merge output as detail; lock released. (Conflict *resolution*
  is deferred.)
- `ctx` cancellation or `Lock.Acquire` error → `Integrating`/`Stage1Passed →
  Cancelled` (no merge attempted).
- The lock release runs via `defer` on every path, including panic.
- Misconfiguration (some but not all of `Lock`/`Stage2`/`Merger` set) → `New`
  panics with a clear message (fail-fast on a wiring bug; `New`'s signature is
  unchanged from slice 1).

## Testing strategy (TDD — tests first)

Unit tests use fakes; no real Postgres or git in the fast path.

- **Fake `IntegrationLock`**: a buffered-channel semaphore (size 1) plus an
  atomic counter; tests assert the counter never exceeds 1 inside the integration
  section (seriality) and that release admits the next worker.
- **Supervisor integration drive**:
  - Stage-2 pass → state sequence `... → Integrating → Merged`, a `base_updated`
    event carrying the SHA, and exactly one `MergeNoFF` call for the worker's branch;
  - Stage-2 clean red → `Failed`, no merge call, lock released;
  - Stage-2 runner error → `Failed` with error detail;
  - merge failure → `Failed` with merge output, lock released;
  - seriality: several workers pass Stage-1, only one integrates at a time;
  - backward-compat: nil integration deps → worker stops at `Stage1Passed`, no
    lock/stage2/merge interaction.
- **Smoke (gated `!testing.Short()`)**: real `policy.GuardedGit` merging a
  worker branch into base in a temp repo, proving the merge wiring end-to-end.
- **Postgres-lock smoke (gated on `DATABASE_URL`)**: acquire/serialize/release
  against a real Postgres via `pg_advisory_lock`; skip when `DATABASE_URL` is
  unset, mirroring the memory package's Postgres-test convention.

## Definition of done

- `internal/controller` compiles and `make test` passes.
- The merge side effect is routed through `*policy.GuardedGit`; the Postgres lock
  is documented as guard-exempt by the `internal/health` precedent.
- No new tables (advisory lock only); four-table schema intact.
- A one-paragraph note added to `docs/Architecture.md`.
- Per-phase commits: (1) lock interface + fake + pg impl, (2) stage2/merger
  seams, (3) supervisor integration drive, (4) smoke tests, (5) Architecture.md.

## Open questions deferred to later slices

- `base_updated` subscribers (propagation, mock-update, re-test, relevance).
- Integration entry-ticket / current-with-base re-sync.
- Code-conflict resolution policy; real-env state reset between serial runs.
- Stage-2 failure → regression-test re-entry loop.
