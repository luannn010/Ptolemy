# Controller — Worker-Pool Supervisor, Lifecycle & Event Bus

**Date:** 2026-06-07
**Status:** Approved (brainstorming) — pending implementation plan
**Slice:** 1 of N for the Ptolemy multi-agent orchestration layer

## Context

Ptolemy v2's platform plumbing (git worktrees, the `Guarded*` harness, memory/RAG,
HTTP, MCP, health) is built and production-quality. The orchestration layer
described in `docs/ptolemy-architecture.html` — the part that coordinates multiple
coding agents running in parallel sandboxes — is entirely absent.

The architecture's build order is:

> worktree + stack lifecycle → review artifacts → mock layer + Stage 1 →
> integration lock + Stage 2 → base_updated propagation → relevance receiver →
> **wire in the model last, on proven plumbing**

and its governing principle is **"keep the model out of the plumbing"**: git,
Docker, locks, tests, and event dispatch are deterministic Go; the model issues
intents *on top of* a controller that never calls a model.

This spec covers the **first deterministic slice**: the spine that every later
slice (parallel dispatch, Stage-1/Stage-2 promotion, `base_updated` propagation,
relevance) hangs off — a worker-pool supervisor, a worker lifecycle state machine,
and an in-process event bus. No Docker, no mocks, no model calls.

Each subsequent slice gets its own spec → plan → TDD cycle.

## Goals

- A deterministic, model-free `internal/controller` package.
- Spawn N workers, each bound to its own git worktree, and drive each through a
  lifecycle via an injected `Runner` (so the real model-backed runner can land
  later without reworking the supervisor).
- An in-process typed event bus so future subscribers (propagation, re-test,
  relevance) attach without modifying the supervisor.
- Full test coverage written test-first (TDD).

## Non-goals (deferred to later slices)

- Docker / `docker compose -p` per-worker stacks.
- HTTP mocks, OpenAPI contracts, Stage-1-against-mock semantics.
- Integration lock, Stage-2-against-real-env, merge to base.
- `base_updated` fan-out subscribers (code propagation, mock-update sender,
  re-test trigger), relevance filter.
- Wiring in the model / orchestrator.
- Persisting worker state to Postgres (this slice keeps the registry in memory;
  the four-table schema is unchanged per CLAUDE.md).

## Architecture & boundaries

Package `internal/controller`, three independently testable units.

| Unit | File | Side effects | Guard? |
|------|------|--------------|--------|
| Event bus | `bus.go` | none (pure in-process) | no |
| Worker + state machine | `worker.go` | none (pure data/logic) | no |
| Supervisor / pool | `supervisor.go` | worktree create/remove | **only via injected `*policy.GuardedWorktree`** |
| Runner interface | `runner.go` | none (interface + value types) | n/a |

**Naming.** The architecture splits the control plane into *Orchestrator* (the
model — wired last) and *Controller* (deterministic git/docker/lock plumbing).
This slice is pure deterministic plumbing, so it lands as `internal/controller`.
This avoids colliding with the existing `memory.Orchestrator`, an unrelated
RAG concept.

**Harness compliance.** The supervisor performs side effects *only* through an
injected `*policy.GuardedWorktree` (and, in later slices, `GuardedGit` /
`GuardedRunner`) — never raw adapters. The bus and state machine are pure and
need no guard. No new DB tables are introduced.

## Components

### Event bus (`bus.go`)

Sync fan-out with a per-subscriber goroutine. `Publish` never blocks the
publisher: each subscriber owns a bounded buffered channel drained on its own
goroutine; on a full buffer the event is **dropped and a dropped-counter
incremented** (observable for tests) rather than blocking.

```go
type EventType string

const (
    EventWorkerStateChanged EventType = "worker.state_changed"
    EventBaseUpdated        EventType = "base_updated" // reserved for a later slice
)

type Event struct {
    Type     EventType
    WorkerID string
    From     State // for state_changed
    To       State // for state_changed
    Payload  any
}

type Bus struct { /* ... */ }

func NewBus() *Bus
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) // ch + unsubscribe
func (b *Bus) Publish(ev Event)                            // non-blocking
func (b *Bus) Dropped() int                                // total dropped across subs
func (b *Bus) Close()                                      // idempotent; closes all subs
```

### Worker + state machine (`worker.go`)

Full lifecycle skeleton modelled now to avoid enum churn later; this slice only
*drives* up to `Stage1Passed`.

```
Pending → Provisioning → Running → Stage1Passed → Integrating → Merged
   (every non-terminal state) → Failed | Cancelled
```

```go
type State string

const (
    StatePending      State = "pending"
    StateProvisioning State = "provisioning"
    StateRunning      State = "running"
    StateStage1Passed State = "stage1_passed"
    StateIntegrating  State = "integrating" // legal but unused this slice
    StateMerged       State = "merged"      // legal but unused this slice
    StateFailed       State = "failed"
    StateCancelled    State = "cancelled"
)

type Worker struct {
    ID       string
    Spec     WorkSpec
    Worktree string // path, set after provisioning
    Branch   string
    State    State
    Detail   string // last outcome/error detail
}

// CanTransition reports whether from→to is legal per the transition table.
func CanTransition(from, to State) bool
```

Terminal states: `Merged`, `Failed`, `Cancelled` (no outgoing transitions).
An illegal transition is rejected by the supervisor with an error and causes no
state mutation and no event.

### Runner interface (`runner.go`)

Injected; the real model-backed implementation arrives in a later slice.

```go
type Outcome struct {
    Passed bool
    Detail string
}

type Runner interface {
    RunStage1(ctx context.Context, w Worker) (Outcome, error)
}
```

`Outcome{Passed: false}` is a clean red (→ `Failed` with detail); a non-nil
`error` is an execution fault (→ `Failed` with the error detail). Both are
terminal for this slice but distinguishable via `Detail`.

### Supervisor / pool (`supervisor.go`)

```go
type WorkSpec struct {
    Name   string // worker/worktree name
    Branch string // optional; defaults per worktree.Manager rules
}

type Config struct {
    MaxWorkers int // concurrency bound, e.g. 4–8; defaults to a sane value if 0
}

type Deps struct {
    Worktree *policy.GuardedWorktree
    Runner   Runner
    Bus      *Bus
    Config   Config
}

func New(deps Deps) *Supervisor

func (s *Supervisor) Spawn(ctx context.Context, sessionID string, spec WorkSpec) (string, error)
func (s *Supervisor) Run(ctx context.Context) error // drives all spawned workers, bounded by MaxWorkers
func (s *Supervisor) Worker(id string) (Worker, bool)
func (s *Supervisor) Workers() []Worker
func (s *Supervisor) Shutdown(ctx context.Context)   // cancel in-flight, remove worktrees, → Cancelled
```

The registry is an in-memory map guarded by a mutex. `Run` provisions and drives
each spawned worker concurrently up to `MaxWorkers`; it returns when all reach a
terminal-for-this-slice state (`Stage1Passed`, `Failed`, or `Cancelled`).

## Data flow (happy path)

1. `Spawn` registers `Worker{State: Pending}`, publishes nothing yet.
2. `Run` picks up the worker, calls `GuardedWorktree.Create` → transition to
   `Provisioning` (event), records the worktree path/branch.
3. Calls `Runner.RunStage1` → transition to `Running` (event).
4. `Outcome.Passed` → `Stage1Passed` (event); else → `Failed` (event).

Every transition publishes a `worker.state_changed` event carrying `From`/`To`,
so future subscribers attach without touching the supervisor.

## Error handling

- Worktree create fails → worker → `Failed` (event); other workers unaffected.
- Runner returns a non-nil error → `Failed` with the error detail (distinct from
  a clean red `Outcome{Passed:false}`).
- `ctx` cancellation / `Shutdown` → in-flight workers → `Cancelled`; their
  worktrees are removed via `GuardedWorktree.Remove`.
- Illegal transition request → error returned to caller, no mutation, no event.
- The bus never blocks the publisher: a stuck subscriber loses events (counted),
  it cannot stall the supervisor.

## Testing strategy (TDD — tests first)

Unit tests use fakes; no real git in the fast path.

- **Bus**: fan-out to N subscribers; unsubscribe stops delivery; a slow/full
  subscriber drops events (counter increments) without blocking the publisher;
  `Close` is idempotent and safe; published-after-close is a no-op.
- **State machine**: table-driven over every legal transition; every illegal
  transition rejected; terminal states have no outgoing transitions.
- **Supervisor**: fake `Runner` + in-memory fake satisfying the worktree
  interface →
  - happy path: assert exact state sequence and ordered events;
  - `MaxWorkers` concurrency bound respected (e.g. via a counting/barrier fake);
  - a `Failed` worker does not affect siblings;
  - Runner error vs clean red are distinguishable in `Detail`;
  - `Shutdown` cancels in-flight workers and removes their worktrees.
- **Integration smoke** (gated behind `!testing.Short()`): a real temp git repo +
  real `policy.GuardedWorktree` proves the wiring end-to-end for the happy path.

To keep the supervisor unit-testable against a fake worktree while respecting the
harness rule (services hold `Guarded*` types, never raw adapters), the supervisor
defines a small consumer-side interface it depends on:

```go
type WorktreeManager interface {
    Create(ctx context.Context, sessionID, name, branch string, opts policy.CallOpts) (worktree.Result, error)
    Remove(ctx context.Context, sessionID, name string, opts policy.CallOpts) (worktree.Result, error)
}
```

`*policy.GuardedWorktree` satisfies this interface as-is (its method set is a
superset). Production wiring in `cmd/workerd/main.go` injects the real
`*policy.GuardedWorktree`; unit tests inject an in-memory fake. No raw adapter is
ever reachable from the supervisor.

## Definition of done

- `internal/controller` compiles in the module and `make test` passes.
- All side effects routed through `Guarded*` (here `GuardedWorktree`).
- A one-paragraph note added to `docs/Architecture.md`.
- Per-phase commits: (1) bus, (2) worker/state machine, (3) supervisor + runner,
  (4) Architecture.md note.

## Open questions for later slices (not this one)

- Code-conflict policy (human resolves vs worker re-tasked).
- Schema/migration conflicts (allow in v1 vs defer).
- Integration lock lease/timeout; real-env state reset between serial runs.
- Whether worker state eventually persists to Postgres (would need a written
  plan + schema change).
