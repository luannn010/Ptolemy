# Deep `/health` Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace workerd's static `/health` stub with a deep readiness probe that reports the health of Brain, Embedder, Workerd (self + SQLite), Postgres, and MCP, returning HTTP 503 when a required dependency is down.

**Architecture:** A new isolated `internal/health` package defines a `Checker` interface; one checker per dependency. An `Aggregator` runs them in parallel under a per-check timeout and composes an overall status + HTTP code. `internal/httpapi` renders the report; `cmd/workerd/main.go` builds the checkers and an optional `pgxpool`. All checks are read-only probes — no guarded adapter is touched.

**Tech Stack:** Go 1.25, `chi/v5` router, `jackc/pgx/v5/pgxpool` (already a dependency), `net/http`, `database/sql`.

Design spec: [docs/superpowers/specs/2026-06-04-health-endpoint-design.md](../specs/2026-06-04-health-endpoint-design.md)

---

## File Structure

- **Create** `internal/health/health.go` — `Status`, `Check`, `Report`, `Checker`, `Aggregator.Run`, `overall`.
- **Create** `internal/health/http_checker.go` — `httpChecker` + `NewHTTPChecker` (Brain, Embedder, MCP).
- **Create** `internal/health/ping_checker.go` — `pingChecker`/`NewSQLChecker` (Workerd SQLite) + `pgChecker`/`NewPgChecker` (Postgres).
- **Create** `internal/health/health_test.go`, `http_checker_test.go`, `ping_checker_test.go`.
- **Modify** `internal/httpapi/router.go` — add `Health` to `RouterDeps`; deep `/health` handler with nil fallback.
- **Modify** `internal/httpapi/router_test.go` — deep-health tests via exported constructors.
- **Modify** `cmd/workerd/main.go` — optional `pgxpool`, build checkers, wire `Aggregator`, close pool on shutdown.
- **Modify** `docs/Architecture.md` — one-paragraph note.

---

## Task 1: health package core — types, Aggregator, overall

**Files:**
- Create: `internal/health/health.go`
- Test: `internal/health/health_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/health/health_test.go`:

```go
package health

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type fakeChecker struct{ c Check }

func (f fakeChecker) Name() string                       { return f.c.Name }
func (f fakeChecker) Check(_ context.Context) Check       { return f.c }

func TestOverall(t *testing.T) {
	cases := []struct {
		name       string
		checks     []Check
		wantStatus string
		wantCode   int
	}{
		{
			name:       "all up",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusUp, Required: false}},
			wantStatus: "ok",
			wantCode:   http.StatusOK,
		},
		{
			name:       "required down",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusDown, Required: true}},
			wantStatus: "unhealthy",
			wantCode:   http.StatusServiceUnavailable,
		},
		{
			name:       "optional down",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusDown, Required: false}},
			wantStatus: "degraded",
			wantCode:   http.StatusOK,
		},
		{
			name:       "optional disabled is fine",
			checks:     []Check{{Status: StatusUp, Required: true}, {Status: StatusDisabled, Required: false}},
			wantStatus: "ok",
			wantCode:   http.StatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotCode := overall(tc.checks)
			if gotStatus != tc.wantStatus || gotCode != tc.wantCode {
				t.Fatalf("overall() = (%q,%d), want (%q,%d)", gotStatus, gotCode, tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestAggregatorRun(t *testing.T) {
	agg := &Aggregator{
		Timeout: 100 * time.Millisecond,
		Checkers: []Checker{
			fakeChecker{Check{Name: "a", Status: StatusUp, Required: true}},
			fakeChecker{Check{Name: "b", Status: StatusDown, Required: true}},
		},
	}
	report, code := agg.Run(context.Background())
	if code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", code)
	}
	if report.Status != "unhealthy" || report.Service != "workerd" {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Checks) != 2 || report.Checks[0].Name != "a" || report.Checks[1].Name != "b" {
		t.Fatalf("checks order wrong: %+v", report.Checks)
	}
	if report.Timestamp == "" {
		t.Fatal("timestamp empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/health/ -run 'TestOverall|TestAggregatorRun' -v`
Expected: FAIL — `undefined: Check`, `undefined: overall`, `undefined: Aggregator`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/health/health.go`:

```go
// Package health provides a deep readiness probe for workerd's dependencies.
// Each dependency is a Checker; an Aggregator runs them in parallel and composes
// an overall status and HTTP code. All checks are read-only — no guarded adapter
// is touched, so this package needs no Guarded* wrapper.
package health

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDisabled Status = "disabled"
)

// Check is one dependency's result.
type Check struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	Required  bool   `json:"required"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Checker probes a single dependency. Implementations must not block past the
// context deadline the Aggregator supplies.
type Checker interface {
	Name() string
	Check(ctx context.Context) Check
}

// Report is the rendered /health body.
type Report struct {
	Status    string  `json:"status"` // ok | degraded | unhealthy
	Service   string  `json:"service"`
	Timestamp string  `json:"timestamp"`
	Checks    []Check `json:"checks"`
}

// Aggregator runs all checkers in parallel under a per-check timeout.
type Aggregator struct {
	Checkers []Checker
	Timeout  time.Duration
}

// Run probes every checker concurrently and returns the composed report and the
// HTTP status code the handler should write.
func (a *Aggregator) Run(ctx context.Context) (Report, int) {
	checks := make([]Check, len(a.Checkers))
	var wg sync.WaitGroup
	for i, c := range a.Checkers {
		wg.Add(1)
		go func(i int, c Checker) {
			defer wg.Done()
			cctx := ctx
			if a.Timeout > 0 {
				var cancel context.CancelFunc
				cctx, cancel = context.WithTimeout(ctx, a.Timeout)
				defer cancel()
			}
			checks[i] = c.Check(cctx)
		}(i, c)
	}
	wg.Wait()

	status, code := overall(checks)
	return Report{
		Status:    status,
		Service:   "workerd",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}, code
}

// overall maps a set of checks to an aggregate status string and HTTP code:
// any required check down -> unhealthy/503; else any optional down -> degraded/200;
// else ok/200. Disabled checks never degrade the aggregate.
func overall(checks []Check) (string, int) {
	degraded := false
	for _, c := range checks {
		if c.Status != StatusDown {
			continue
		}
		if c.Required {
			return "unhealthy", http.StatusServiceUnavailable
		}
		degraded = true
	}
	if degraded {
		return "degraded", http.StatusOK
	}
	return "ok", http.StatusOK
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/health/ -run 'TestOverall|TestAggregatorRun' -v`
Expected: PASS (both tests, all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/health/health.go internal/health/health_test.go
git commit -m "feat(health): core types, parallel aggregator, status logic"
```

---

## Task 2: httpChecker (Brain, Embedder, MCP)

**Files:**
- Create: `internal/health/http_checker.go`
- Test: `internal/health/http_checker_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/health/http_checker_test.go`:

```go
package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPChecker_Up(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPChecker("brain", srv.URL, "/v1/models", true).Check(context.Background())
	if c.Status != StatusUp || c.Name != "brain" || !c.Required {
		t.Fatalf("got %+v, want up/brain/required", c)
	}
}

func TestHTTPChecker_DownOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewHTTPChecker("brain", srv.URL, "/v1/models", true).Check(context.Background())
	if c.Status != StatusDown || c.Error == "" {
		t.Fatalf("got %+v, want down with error", c)
	}
}

func TestHTTPChecker_DownOnConnRefused(t *testing.T) {
	// Reserve a port then close it so the dial is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := NewHTTPChecker("embedder", url, "/v1/models", true).Check(context.Background())
	if c.Status != StatusDown {
		t.Fatalf("got %+v, want down", c)
	}
}

func TestHTTPChecker_EmptyURL_RequiredDown(t *testing.T) {
	c := NewHTTPChecker("embedder", "", "/v1/models", true).Check(context.Background())
	if c.Status != StatusDown || c.Error != "not configured" {
		t.Fatalf("got %+v, want down/not configured", c)
	}
}

func TestHTTPChecker_EmptyURL_OptionalDisabled(t *testing.T) {
	c := NewHTTPChecker("mcp", "", "/health", false).Check(context.Background())
	if c.Status != StatusDisabled {
		t.Fatalf("got %+v, want disabled", c)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/health/ -run TestHTTPChecker -v`
Expected: FAIL — `undefined: NewHTTPChecker`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/health/http_checker.go`:

```go
package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// httpChecker probes an OpenAI-compatible or MCP HTTP endpoint with a GET and
// treats any 2xx as healthy. An empty baseURL means "not configured": a required
// checker reports down, an optional one reports disabled.
type httpChecker struct {
	name     string
	baseURL  string
	path     string
	required bool
	client   *http.Client
}

// NewHTTPChecker builds a GET-based liveness checker. Used for Brain and Embedder
// (path "/v1/models") and MCP (path "/health").
func NewHTTPChecker(name, baseURL, path string, required bool) Checker {
	return &httpChecker{
		name:     name,
		baseURL:  baseURL,
		path:     path,
		required: required,
		client:   &http.Client{},
	}
}

func (h *httpChecker) Name() string { return h.name }

func (h *httpChecker) Check(ctx context.Context) Check {
	c := Check{Name: h.name, Required: h.required}
	if h.baseURL == "" {
		if h.required {
			c.Status = StatusDown
			c.Error = "not configured"
		} else {
			c.Status = StatusDisabled
		}
		return c
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+h.path, nil)
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	resp, err := h.client.Do(req)
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.Status = StatusDown
		c.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return c
	}
	c.Status = StatusUp
	return c
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/health/ -run TestHTTPChecker -v`
Expected: PASS (all five subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/health/http_checker.go internal/health/http_checker_test.go
git commit -m "feat(health): http checker for brain, embedder, mcp"
```

---

## Task 3: ping & pg checkers (Workerd SQLite, Postgres)

**Files:**
- Create: `internal/health/ping_checker.go`
- Test: `internal/health/ping_checker_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/health/ping_checker_test.go`:

```go
package health

import (
	"context"
	"errors"
	"testing"
)

type fakeSQL struct{ err error }

func (f fakeSQL) PingContext(_ context.Context) error { return f.err }

type fakePG struct{ err error }

func (f fakePG) Ping(_ context.Context) error { return f.err }

func TestSQLChecker_Up(t *testing.T) {
	c := NewSQLChecker("workerd", fakeSQL{nil}, true).Check(context.Background())
	if c.Status != StatusUp || !c.Required {
		t.Fatalf("got %+v, want up/required", c)
	}
}

func TestSQLChecker_Down(t *testing.T) {
	c := NewSQLChecker("workerd", fakeSQL{errors.New("boom")}, true).Check(context.Background())
	if c.Status != StatusDown || c.Error != "boom" {
		t.Fatalf("got %+v, want down/boom", c)
	}
}

func TestSQLChecker_NilDown(t *testing.T) {
	c := NewSQLChecker("workerd", nil, true).Check(context.Background())
	if c.Status != StatusDown || c.Error != "not configured" {
		t.Fatalf("got %+v, want down/not configured", c)
	}
}

func TestPgChecker_Up(t *testing.T) {
	c := NewPgChecker("postgres", fakePG{nil}).Check(context.Background())
	if c.Status != StatusUp || c.Required {
		t.Fatalf("got %+v, want up/optional", c)
	}
}

func TestPgChecker_Down(t *testing.T) {
	c := NewPgChecker("postgres", fakePG{errors.New("refused")}).Check(context.Background())
	if c.Status != StatusDown || c.Error != "refused" {
		t.Fatalf("got %+v, want down/refused", c)
	}
}

func TestPgChecker_NilDisabled(t *testing.T) {
	c := NewPgChecker("postgres", nil).Check(context.Background())
	if c.Status != StatusDisabled {
		t.Fatalf("got %+v, want disabled", c)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/health/ -run 'SQLChecker|PgChecker' -v`
Expected: FAIL — `undefined: NewSQLChecker`, `undefined: NewPgChecker`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/health/ping_checker.go`:

```go
package health

import (
	"context"
	"time"
)

// sqlPinger is satisfied by *sql.DB (PingContext).
type sqlPinger interface {
	PingContext(ctx context.Context) error
}

// pgPinger is satisfied by *pgxpool.Pool (Ping).
type pgPinger interface {
	Ping(ctx context.Context) error
}

// pingChecker probes a *sql.DB handle (workerd's own SQLite store). It is
// required: a nil handle is a misconfiguration and reports down.
type pingChecker struct {
	name     string
	db       sqlPinger
	required bool
}

// NewSQLChecker builds a PingContext-based checker over a *sql.DB.
func NewSQLChecker(name string, db sqlPinger, required bool) Checker {
	return &pingChecker{name: name, db: db, required: required}
}

func (p *pingChecker) Name() string { return p.name }

func (p *pingChecker) Check(ctx context.Context) Check {
	c := Check{Name: p.name, Required: p.required}
	if p.db == nil {
		c.Status = StatusDown
		c.Error = "not configured"
		return c
	}
	start := time.Now()
	err := p.db.PingContext(ctx)
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	c.Status = StatusUp
	return c
}

// pgChecker probes a Postgres pool. It is optional: a nil pool means the memory
// DB is not configured and reports disabled.
type pgChecker struct {
	name string
	pool pgPinger
}

// NewPgChecker builds a Ping-based checker over a *pgxpool.Pool. Pass a nil
// pgPinger (not a typed-nil pool) when DATABASE_URL is unset.
func NewPgChecker(name string, pool pgPinger) Checker {
	return &pgChecker{name: name, pool: pool}
}

func (p *pgChecker) Name() string { return p.name }

func (p *pgChecker) Check(ctx context.Context) Check {
	c := Check{Name: p.name, Required: false}
	if p.pool == nil {
		c.Status = StatusDisabled
		return c
	}
	start := time.Now()
	err := p.pool.Ping(ctx)
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	c.Status = StatusUp
	return c
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/health/ -v`
Expected: PASS (all health package tests).

- [ ] **Step 5: Commit**

```bash
git add internal/health/ping_checker.go internal/health/ping_checker_test.go
git commit -m "feat(health): sql and postgres ping checkers"
```

---

## Task 4: wire deep /health into the router

**Files:**
- Modify: `internal/httpapi/router.go` (imports, `RouterDeps`, `/health` handler at lines 16-30)
- Test: `internal/httpapi/router_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/httpapi/router_test.go` (top-level functions; reuse existing imports plus add `"github.com/luannn010/ptolemy/internal/health"` and `"net/http/httptest"` — `net/http`, `encoding/json`, `context` are already imported):

```go
func TestHealth_DeepOK(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	agg := &health.Aggregator{Checkers: []health.Checker{
		health.NewHTTPChecker("brain", up.URL, "/v1/models", true),
		health.NewHTTPChecker("embedder", up.URL, "/v1/models", true),
		health.NewPgChecker("postgres", nil),                 // disabled
		health.NewHTTPChecker("mcp", "", "/health", false),   // disabled
	}}
	router := NewRouter(RouterDeps{Health: agg})

	srv := httptest.NewServer(router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code = %d, want 200", resp.StatusCode)
	}
	var report health.Report
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "ok" || report.Service != "workerd" || len(report.Checks) != 4 {
		t.Fatalf("report = %+v", report)
	}
}

func TestHealth_RequiredDown503(t *testing.T) {
	agg := &health.Aggregator{Checkers: []health.Checker{
		health.NewHTTPChecker("brain", "", "/v1/models", true), // required, not configured -> down
	}}
	router := NewRouter(RouterDeps{Health: agg})

	srv := httptest.NewServer(router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", resp.StatusCode)
	}
}

func TestHealth_NilFallback(t *testing.T) {
	router := NewRouter(RouterDeps{}) // no Health wired
	srv := httptest.NewServer(router)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code = %d, want 200", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/httpapi/ -run TestHealth -v`
Expected: FAIL — `unknown field Health in struct literal` / `undefined: health`.

- [ ] **Step 3: Write minimal implementation**

In `internal/httpapi/router.go`, add the import (alongside the existing internal imports):

```go
	"github.com/luannn010/ptolemy/internal/health"
```

Add the field to `RouterDeps`:

```go
type RouterDeps struct {
	Sessions  *session.Store
	Commands  *command.Service
	CommandDB *command.Store
	Health    *health.Aggregator
}
```

Replace the existing `/health` handler (currently lines 24-30) with:

```go
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		if deps.Health == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    "ok",
				"service":   "workerd",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		report, code := deps.Health.Run(req.Context())
		writeJSON(w, code, report)
	})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS — `TestHealth_DeepOK`, `TestHealth_RequiredDown503`, `TestHealth_NilFallback`, and all pre-existing router tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/router.go internal/httpapi/router_test.go
git commit -m "feat(httpapi): deep /health report with nil fallback"
```

---

## Task 5: wire checkers + Postgres pool into workerd

**Files:**
- Modify: `cmd/workerd/main.go` (imports; build pool + aggregator before the `server` literal at lines 67-77; close pool in shutdown at lines 120-127)

- [ ] **Step 1: Add imports**

In `cmd/workerd/main.go`, add to the import block:

```go
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luannn010/ptolemy/internal/health"
```

(`context` and `time` are already imported.)

- [ ] **Step 2: Build the pool and aggregator**

Insert immediately before the `server := &http.Server{` line (currently line 67):

```go
	// Optional Postgres pool for the memory DB. pgxpool.New is lazy — it does not
	// dial here, so an unreachable DB surfaces only at /health Ping time, not at
	// startup. nil pool => Postgres reports "disabled".
	var pgPool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		pool, perr := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if perr != nil {
			log.Warn().Err(perr).Msg("postgres pool init failed; /health will report postgres down")
		} else {
			pgPool = pool
		}
	}
	pgCheck := health.NewPgChecker("postgres", nil)
	if pgPool != nil {
		pgCheck = health.NewPgChecker("postgres", pgPool)
	}
	healthAgg := &health.Aggregator{
		Timeout: time.Duration(cfg.HealthTimeoutMS) * time.Millisecond,
		Checkers: []health.Checker{
			health.NewSQLChecker("workerd", baseStore.SQLDB(), true),
			health.NewHTTPChecker("brain", cfg.BrainBaseURL, "/v1/models", true),
			health.NewHTTPChecker("embedder", cfg.EmbeddingBaseURL, "/v1/models", true),
			pgCheck,
			health.NewHTTPChecker("mcp", cfg.MCPBaseURL, "/health", false),
		},
	}
```

- [ ] **Step 3: Pass the aggregator into RouterDeps**

Change the `httpapi.NewRouter` call (currently lines 69-73) to include `Health`:

```go
		Handler: httpapi.NewRouter(httpapi.RouterDeps{
			Sessions:  sessionStore,
			Commands:  commandService,
			CommandDB: commandStore,
			Health:    healthAgg,
		}),
```

- [ ] **Step 4: Close the pool on shutdown**

In the shutdown section, after `if sweepCleanup != nil { sweepCleanup() }` (currently lines 120-122), add:

```go
	if pgPool != nil {
		pgPool.Close()
	}
```

- [ ] **Step 5: Build and verify**

Run: `go build ./... && go vet ./cmd/workerd/ ./internal/health/ ./internal/httpapi/`
Expected: no output (clean build + vet).

Run: `go test ./internal/health/ ./internal/httpapi/ -count=1`
Expected: PASS for both packages.

- [ ] **Step 6: Commit**

```bash
git add cmd/workerd/main.go go.mod go.sum
git commit -m "feat(workerd): wire health aggregator + optional postgres pool"
```

(`go.mod`/`go.sum` may change if `pgxpool` moves from indirect to direct; include them only if `git status` shows them modified.)

---

## Task 6: architecture note + final verification

**Files:**
- Modify: `docs/Architecture.md` (append a paragraph)

- [ ] **Step 1: Add the Architecture note**

Append to `docs/Architecture.md`:

```markdown
## Health endpoint (`internal/health`)

workerd's `GET /health` is a deep readiness probe. The `internal/health` package
defines a `Checker` interface and an `Aggregator` that runs one checker per
dependency in parallel under a per-check timeout (`HEALTH_TIMEOUT_MS`, default
1500ms). Brain and Embedder are probed with `GET /v1/models`; the Workerd line
pings its own SQLite store; Postgres (memory DB) is pinged via a lazily-opened
`pgxpool`; MCP is probed with `GET /health`. Brain, Embedder, and Workerd are
required — any one down yields overall `unhealthy` and HTTP 503. Postgres and MCP
are optional — down yields `degraded` (200) and an unset endpoint yields `disabled`
(200). All checks are read-only probes that touch no workspace, shell, or git, so
the package needs no `Guarded*` wrapper, consistent with the harness rules.
```

- [ ] **Step 2: Full build + targeted tests**

Run: `go build ./...`
Expected: clean.

Run: `go test ./internal/health/ ./internal/httpapi/ -count=1 -v`
Expected: all PASS.

- [ ] **Step 3: Manual smoke (optional, requires services)**

Run (workerd in one shell): `make build && ./bin/workerd`
Then: `curl -s localhost:8080/health | jq` (or `Invoke-RestMethod http://localhost:8080/health` in PowerShell)
Expected: JSON report with `status`, `service: "workerd"`, and five `checks` entries. With Brain/Embedder reachable and `DATABASE_URL`/`MCP_BASE_URL` set, all up → HTTP 200; stop Brain → `brain` down → HTTP 503.

- [ ] **Step 4: Commit**

```bash
git add docs/Architecture.md
git commit -m "docs(health): architecture note for deep /health probe"
```

---

## Self-Review Notes

- **Spec coverage:** route shape (deep `/health`, Task 4) ✓; required/optional + 503 (`overall`, Task 1; wiring Task 5) ✓; Workerd self+SQLite (`NewSQLChecker`, Tasks 3/5) ✓; Brain/Embedder `GET /v1/models` (Task 2/5) ✓; MCP `GET /health` (Task 2/5) ✓; Postgres pgxpool at startup, ping per call, disabled when unset (Tasks 3/5) ✓; required-but-unconfigured → down/503 (`httpChecker` empty-URL branch + `TestHTTPChecker_EmptyURL_RequiredDown`) ✓; nil-`Health` fallback (Task 4) ✓; Architecture note (Task 6) ✓.
- **Placeholder scan:** none — every code step shows complete code.
- **Type consistency:** `Check`, `Status`, `Report`, `Checker`, `Aggregator`, `overall`, `NewHTTPChecker`, `NewSQLChecker`, `NewPgChecker`, `RouterDeps.Health` used identically across tasks.
- **Nil-interface gotcha:** `NewPgChecker("postgres", nil)` passes an untyped nil so the `p.pool == nil` guard fires (disabled); Task 5 only swaps in `pgPool` when non-nil — never a typed-nil pool.
