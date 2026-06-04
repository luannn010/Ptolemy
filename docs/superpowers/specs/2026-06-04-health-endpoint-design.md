# Deep `/health` endpoint — design

Date: 2026-06-04
Branch: `ptolemy/health-endpoint`
Status: approved, pending implementation plan

## Goal

Turn the workerd `/health` route from a static `{"status":"ok"}` stub into a deep
readiness probe that reports the health of every service workerd depends on:
**Brain**, **Embedder**, **Workerd** (self + its SQLite store), **Postgres** (memory
DB), and **MCP**.

## Decisions (locked)

- **Route shape:** make the existing `/health` route itself deep — no separate
  `/readyz`. One surface; every probe reports the full dependency report.
- **Required vs optional:**
  - Required: **Brain**, **Embedder**, **Workerd/SQLite**. A required check `down`
    makes the overall status `unhealthy` and returns **HTTP 503**.
  - Optional: **Postgres**, **MCP**. When down they yield `degraded` (HTTP 200);
    when their endpoint is unconfigured they report `disabled` (HTTP 200). This
    matches the codebase stance that memory/MCP are simply *absent* when their env
    vars are unset.
- **Workerd line item:** `self + SQLite`. workerd is trivially up (it answered the
  request); the meaningful signal is pinging its own SQLite store (`cfg.DBPath` via
  `baseStore.SQLDB()`).
- **Brain/Embedder probe:** `GET /v1/models` — the cheap OpenAI-compatible liveness
  probe (no tokens, no model load). Healthy = HTTP 2xx.
- **MCP probe:** `GET /health` on `cfg.MCPBaseURL` (the `internal/mcp` WorkerClient
  already has a `Get("/health")`).
- **Postgres probe:** a `pgxpool` opened once at workerd startup when `DATABASE_URL`
  is set; `/health` does a fast `Ping`. Unset → `disabled`. Reuses one connection,
  low per-probe cost.

## Architecture

### New package `internal/health`

Isolated and independently testable — no live services needed for its unit tests.

```go
type Status string // "up" | "down" | "disabled"

const (
    StatusUp       Status = "up"
    StatusDown     Status = "down"
    StatusDisabled Status = "disabled"
)

type Check struct {
    Name      string `json:"name"`
    Status    Status `json:"status"`
    Required  bool   `json:"required"`
    LatencyMS int64  `json:"latency_ms,omitempty"`
    Error     string `json:"error,omitempty"`
}

type Checker interface {
    Name() string
    Check(ctx context.Context) Check
}

type Report struct {
    Status    string  `json:"status"`    // ok | degraded | unhealthy
    Service   string  `json:"service"`   // "workerd"
    Timestamp string  `json:"timestamp"` // RFC3339 UTC
    Checks    []Check `json:"checks"`
}

type Aggregator struct {
    Checkers []Checker
    Timeout  time.Duration // from cfg.HealthTimeoutMS
}

// Run executes every checker in parallel, each under a per-check timeout, and
// composes the overall status. Returns the report and the HTTP status code.
func (a *Aggregator) Run(ctx context.Context) (Report, int)
```

### Checkers (each small, own file)

- `httpChecker` (reusable): `GET baseURL+path`; `up` on 2xx, else `down` with the
  status/err in `Error`. Used for **Brain** (`/v1/models`), **Embedder**
  (`/v1/models`), **MCP** (`/health`).
- `pgChecker`: `pgxpool.Ping` → **Postgres**.
- `pingChecker`: `*sql.DB.PingContext` → **Workerd** (pings its own SQLite store).
- Optional checkers constructed with an empty base URL / nil handle report
  `disabled` and never run a probe.
- **Required** checkers constructed with an empty base URL / nil handle are a
  *misconfiguration*, not a disabled feature: they report `down` (error
  `"not configured"`), which drives the overall status to `unhealthy` / 503. This
  matters because `EMBEDDING_BASE_URL` has no default — an unset Embedder endpoint
  means workerd is genuinely not ready, so it must fail rather than silently pass.
  (Brain has a default base URL, so it is always probed.)

### Overall status / HTTP code

- Any **required** check `down` → `"unhealthy"` → **HTTP 503**.
- No required down, but an **optional** check `down` → `"degraded"` → **HTTP 200**.
- All `up` (`disabled` ignored) → `"ok"` → **HTTP 200**.

### Response shape

```json
{
  "status": "ok",
  "service": "workerd",
  "timestamp": "2026-06-04T00:00:00Z",
  "checks": [
    {"name": "workerd",  "status": "up",       "required": true,  "latency_ms": 0},
    {"name": "brain",    "status": "up",       "required": true,  "latency_ms": 12},
    {"name": "embedder", "status": "up",       "required": true,  "latency_ms": 9},
    {"name": "postgres", "status": "disabled", "required": false},
    {"name": "mcp",      "status": "disabled", "required": false}
  ]
}
```

## Wiring

- `cmd/workerd/main.go`:
  - Open a `pgxpool` when `DATABASE_URL` is set (nil → Postgres `disabled`).
  - Build the five checkers from `cfg`.
  - Construct `*health.Aggregator{Checkers, Timeout: cfg.HealthTimeoutMS}` and pass
    it into `RouterDeps`.
  - Close the pool on shutdown alongside the other cleanup.
- `internal/httpapi`:
  - `RouterDeps` gains `Health *health.Aggregator`.
  - The `/health` handler calls `Health.Run(ctx)` and writes the report with the
    returned status code.
  - If `Health` is nil (existing tests that don't wire it), fall back to the current
    static `{"status":"ok"}` so nothing breaks.

## Testing (TDD, no live services)

- `aggregator_test.go`: table-driven overall-status + HTTP-code from a set of checks
  (all up; required down; optional down; optional disabled).
- `http_checker_test.go`: `httptest` server returning 200 / 500 / connection-refused
  → `up` / `down` / `down`.
- `ping_checker_test.go`: in-memory SQLite `up`; closed DB → `down`. `pgChecker`
  against a closed/unreachable pool → `down`.
- `router_test.go`: inject fake checkers → assert the 200 report shape, and 503 when
  a required checker is down.

## Out of scope (YAGNI)

- Auth on `/health`.
- Caching / rate-limiting probe results.
- Historical health / metrics.
- Per-check configurable required/optional (hard-coded per the decisions above).
- A separate `/readyz` or `/health/services` route.

## Definition of done

- `internal/health` compiles and its unit tests pass.
- `/health` returns the deep report with correct status codes.
- No live service is required for any test.
- One-paragraph note added to `docs/Architecture.md`.
- The new route does not touch a guarded adapter (read-only probes only) — consistent
  with the harness rules; no `Guarded*` wrapper needed because the checks perform no
  side effects on the workspace, shell, or git.
