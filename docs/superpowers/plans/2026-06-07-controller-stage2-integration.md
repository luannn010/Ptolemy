# Controller Stage-2 Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `internal/controller` so a `Stage1Passed` worker is promoted serially — acquire an integration lock, run Stage 2, merge to base via a guarded merger, and emit `base_updated`.

**Architecture:** New injected seams (`IntegrationLock`, `Stage2Runner`, `GitMerger`) keep `pgx` and `git` out of the controller core. The supervisor's `driveWorker` continues past `Stage1Passed` only when all three are configured (else slice-1 behavior). The merge goes through `*policy.GuardedGit`; the Postgres advisory lock is guard-exempt by the `internal/health` precedent. No new tables.

**Tech Stack:** Go 1.25, stdlib + `github.com/jackc/pgx/v5/pgxpool`, module `github.com/luannn010/ptolemy`. Table-driven tests, fakes in `_test.go`, real-git/real-Postgres smokes gated.

**Spec:** `docs/superpowers/specs/2026-06-07-controller-stage2-integration-design.md`

---

## File Structure

- Create `internal/controller/lock.go` — `IntegrationLock` interface.
- Create `internal/controller/lock_test.go` — fake-lock seriality test.
- Create `internal/controller/stage2.go` — `Stage2Runner` + `GitMerger` interfaces.
- Create `internal/controller/pglock.go` — Postgres advisory-lock impl (isolates `pgx`).
- Modify `internal/controller/supervisor.go` — `Deps`/`Config` fields, `integrationConfigured`, `New` panic on partial config, `driveWorker` integration drive.
- Create `internal/controller/integration_test.go` — supervisor integration-drive tests.
- Create `internal/controller/integration_smoke_test.go` — real `GuardedGit` merge smoke + Postgres-lock smoke.
- Modify `docs/Architecture.md` — append a one-paragraph note.

Commit boundaries: (1) lock interface + fake test, (2) stage2/merger seams + pg lock impl, (3) supervisor integration drive, (4) smokes, (5) Architecture.md.

---

## Task 1: Integration lock interface + fake

**Files:**
- Create: `internal/controller/lock.go`
- Test: `internal/controller/lock_test.go`

- [ ] **Step 1: Write the failing test**

```go
package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeLock is an in-memory IntegrationLock: a size-1 semaphore plus a counter
// that records the maximum number of concurrent holders observed.
type fakeLock struct {
	sem     chan struct{}
	holders int32
	maxSeen int32
}

func newFakeLock() *fakeLock { return &fakeLock{sem: make(chan struct{}, 1)} }

func (l *fakeLock) Acquire(ctx context.Context) (func(), error) {
	select {
	case l.sem <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
	n := atomic.AddInt32(&l.holders, 1)
	for {
		old := atomic.LoadInt32(&l.maxSeen)
		if n <= old || atomic.CompareAndSwapInt32(&l.maxSeen, old, n) {
			break
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			atomic.AddInt32(&l.holders, -1)
			<-l.sem
		})
	}, nil
}

func TestFakeLockSerializes(t *testing.T) {
	l := newFakeLock()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := l.Acquire(context.Background())
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			// hold briefly so overlap would be observed if seriality were broken
			for j := 0; j < 1000; j++ {
				_ = j
			}
			rel()
		}()
	}
	wg.Wait()
	if l.maxSeen > 1 {
		t.Fatalf("max concurrent holders = %d, want 1", l.maxSeen)
	}
}

// compile-time assertion that fakeLock implements IntegrationLock.
var _ IntegrationLock = (*fakeLock)(nil)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/controller/ -run TestFakeLockSerializes -v`
Expected: FAIL — `undefined: IntegrationLock`.

- [ ] **Step 3: Write the minimal implementation**

```go
package controller

import "context"

// IntegrationLock serializes Stage-2 promotion: at most one holder at a time.
// The returned release func is idempotent and must be called (defer) to free
// the lock. Acquire blocks until the lock is held or ctx is done.
type IntegrationLock interface {
	Acquire(ctx context.Context) (release func(), err error)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/controller/ -run TestFakeLockSerializes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/lock.go internal/controller/lock_test.go
git commit -m "feat(controller): IntegrationLock interface + in-memory fake"
```

---

## Task 2: Stage2Runner / GitMerger seams + Postgres lock impl

**Files:**
- Create: `internal/controller/stage2.go`
- Create: `internal/controller/pglock.go`

- [ ] **Step 1: Write `stage2.go` (interfaces — no test of their own; exercised in Task 3)**

```go
package controller

import (
	"context"

	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/policy"
)

// Stage2Runner tests a worker against the real environment. Outcome.Passed=false
// is a clean red; a non-nil error is an execution fault. Outcome is reused from
// runner.go (slice 1).
type Stage2Runner interface {
	RunStage2(ctx context.Context, w Worker) (Outcome, error)
}

// GitMerger performs the no-fast-forward merge of a worker's branch into base
// and reports the resulting base commit SHA. Satisfied by *policy.GuardedGit.
type GitMerger interface {
	MergeNoFF(ctx context.Context, sessionID, branch string, opts policy.CallOpts) (gitops.Result, error)
	CurrentCommitSHA(ctx context.Context, sessionID string, opts policy.CallOpts) (gitops.Result, error)
}
```

- [ ] **Step 2: Write `pglock.go` (Postgres advisory-lock implementation)**

```go
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgConnPool is the slice of *pgxpool.Pool the lock needs. Declared as an
// interface so the lock can be tested against a fake pool.
type PgConnPool interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// PgLock is an IntegrationLock backed by a Postgres session-level advisory lock.
// Because a session advisory lock is bound to its connection, a crashed holder's
// connection drops and Postgres releases the lock automatically (crash reclaim).
// A non-zero lease caps how long a live holder may keep the lock by bounding the
// context the caller runs under (enforced by the caller via the returned ctx is
// out of scope here; lease is recorded for callers/telemetry).
type PgLock struct {
	pool  PgConnPool
	key   int64
	lease time.Duration
}

// NewPgLock constructs a Postgres advisory lock over the given pool and 64-bit
// key. lease may be zero (no cap).
func NewPgLock(pool PgConnPool, key int64, lease time.Duration) *PgLock {
	return &PgLock{pool: pool, key: key, lease: lease}
}

// Acquire grabs a dedicated connection and blocks on pg_advisory_lock(key).
func (l *PgLock) Acquire(ctx context.Context) (func(), error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return func() {}, fmt.Errorf("pglock: acquire conn: %w", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", l.key); err != nil {
		conn.Release()
		return func() {}, fmt.Errorf("pglock: advisory_lock: %w", err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		// Use a fresh short context so unlock runs even if the caller's ctx is done.
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(uctx, "SELECT pg_advisory_unlock($1)", l.key)
		conn.Release()
	}, nil
}

// compile-time checks.
var _ IntegrationLock = (*PgLock)(nil)
var _ PgConnPool = (*pgxpool.Pool)(nil)
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/controller/`
Expected: builds with no errors (interfaces + pg impl present; not yet wired into the supervisor).

- [ ] **Step 4: Commit**

```bash
git add internal/controller/stage2.go internal/controller/pglock.go
git commit -m "feat(controller): Stage2Runner/GitMerger seams + Postgres advisory lock"
```

---

## Task 3: Supervisor integration drive

**Files:**
- Modify: `internal/controller/supervisor.go`
- Test: `internal/controller/integration_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package controller

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/policy"
)

// fakeStage2 returns a fixed outcome/error.
type fakeStage2 struct {
	outcome Outcome
	err     error
}

func (f *fakeStage2) RunStage2(ctx context.Context, w Worker) (Outcome, error) {
	return f.outcome, f.err
}

// fakeMerger records merge calls and returns configurable results.
type fakeMerger struct {
	mu         sync.Mutex
	merged     []string
	mergeFail  bool
	sha        string
}

func (m *fakeMerger) MergeNoFF(ctx context.Context, sessionID, branch string, opts policy.CallOpts) (gitops.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mergeFail {
		return gitops.Result{Success: false, Output: "CONFLICT"}, nil
	}
	m.merged = append(m.merged, branch)
	return gitops.Result{Success: true, Output: "merged"}, nil
}

func (m *fakeMerger) CurrentCommitSHA(ctx context.Context, sessionID string, opts policy.CallOpts) (gitops.Result, error) {
	sha := m.sha
	if sha == "" {
		sha = "deadbeef"
	}
	return gitops.Result{Success: true, Output: sha}, nil
}

func integDeps(lock IntegrationLock, s2 Stage2Runner, mg GitMerger, bus *Bus) Deps {
	return Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{outcome: Outcome{Passed: true, Detail: "s1 green"}},
		Bus:      bus,
		Config:   Config{MaxWorkers: 2, BaseBranch: "main"},
		Lock:     lock,
		Stage2:   s2,
		Merger:   mg,
	}
}

func TestIntegrationHappyPathMergesAndEmitsBaseUpdated(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe(64)
	defer unsub()
	evs, waitCollect := collect(ch)

	mg := &fakeMerger{sha: "abc123"}
	s := New(integDeps(newFakeLock(), &fakeStage2{outcome: Outcome{Passed: true}}, mg, bus))
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "alpha"})
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	w, _ := s.Worker(id)
	if w.State != StateMerged {
		t.Fatalf("got state %s, want merged", w.State)
	}
	if len(mg.merged) != 1 {
		t.Fatalf("expected one merge, got %v", mg.merged)
	}
	bus.Close()
	waitCollect()
	var sawBaseUpdated bool
	for _, e := range *evs {
		if e.Type == EventBaseUpdated && e.WorkerID == id {
			sawBaseUpdated = true
			if e.Payload != "abc123" {
				t.Fatalf("base_updated payload = %v, want abc123", e.Payload)
			}
		}
	}
	if !sawBaseUpdated {
		t.Fatal("expected a base_updated event")
	}
	want := []State{StateProvisioning, StateRunning, StateStage1Passed, StateIntegrating, StateMerged}
	if got := statesOf(*evs, id); !equalStates(got, want) {
		t.Fatalf("states = %v, want %v", got, want)
	}
}

func TestIntegrationStage2RedGoesToFailedNoMerge(t *testing.T) {
	mg := &fakeMerger{}
	s := New(integDeps(newFakeLock(), &fakeStage2{outcome: Outcome{Passed: false, Detail: "real env red"}}, mg, NewBus()))
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "beta"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateFailed || w.Detail != "real env red" {
		t.Fatalf("got %+v", w)
	}
	if len(mg.merged) != 0 {
		t.Fatalf("must not merge on stage-2 red, got %v", mg.merged)
	}
}

func TestIntegrationStage2ErrorGoesToFailed(t *testing.T) {
	s := New(integDeps(newFakeLock(), &fakeStage2{err: errors.New("boom")}, &fakeMerger{}, NewBus()))
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "gamma"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateFailed || w.Detail != "boom" {
		t.Fatalf("got %+v", w)
	}
}

func TestIntegrationMergeFailureGoesToFailed(t *testing.T) {
	s := New(integDeps(newFakeLock(), &fakeStage2{outcome: Outcome{Passed: true}}, &fakeMerger{mergeFail: true}, NewBus()))
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "delta"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateFailed {
		t.Fatalf("expected Failed on merge failure, got %+v", w)
	}
}

func TestIntegrationIsSerialAcrossWorkers(t *testing.T) {
	lock := newFakeLock()
	s := New(integDeps(lock, &fakeStage2{outcome: Outcome{Passed: true}}, &fakeMerger{}, NewBus()))
	for _, n := range []string{"a", "b", "c", "d"} {
		s.Spawn(context.Background(), "sess", WorkSpec{Name: n})
	}
	_ = s.Run(context.Background())
	if lock.maxSeen > 1 {
		t.Fatalf("integration not serial: max concurrent = %d", lock.maxSeen)
	}
	for _, w := range s.Workers() {
		if w.State != StateMerged {
			t.Fatalf("worker %s state %s, want merged", w.ID, w.State)
		}
	}
}

func TestIntegrationNilDepsStopsAtStage1Passed(t *testing.T) {
	// slice-1 behavior: no Lock/Stage2/Merger configured.
	s := New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{outcome: Outcome{Passed: true}},
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 1},
	})
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "eps"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateStage1Passed {
		t.Fatalf("got %s, want stage1_passed (no integration configured)", w.State)
	}
}

func TestNewPanicsOnPartialIntegrationConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on partial integration config")
		}
	}()
	// Lock set but Stage2/Merger nil → wiring bug.
	New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{},
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 1},
		Lock:     newFakeLock(),
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/controller/ -run "TestIntegration|TestNewPanics" -v`
Expected: FAIL — `Deps` has no `Lock`/`Stage2`/`Merger` fields; `Config` has no `BaseBranch`.

- [ ] **Step 3: Extend `Config` and `Deps` in `supervisor.go`**

Replace the existing `Config` and `Deps` type declarations with:

```go
// Config tunes the supervisor.
type Config struct {
	MaxWorkers int    // concurrency bound; defaults to 4 when <= 0
	BaseBranch string // merge target; defaults to "main" when empty and integration is configured
}

// Deps are the supervisor's injected dependencies.
type Deps struct {
	Worktree WorktreeManager
	Runner   Runner
	Bus      *Bus
	Config   Config

	// Integration (slice 2). All three nil => slice-1 behavior (stop at
	// Stage1Passed). Setting some-but-not-all panics in New (wiring bug).
	Lock   IntegrationLock
	Stage2 Stage2Runner
	Merger GitMerger
}
```

- [ ] **Step 4: Add config validation to `New` and an `integrationConfigured` helper**

In `supervisor.go`, replace the existing `New` function with:

```go
// New constructs a Supervisor. Panics if integration deps are partially set.
func New(deps Deps) *Supervisor {
	max := deps.Config.MaxWorkers
	if max <= 0 {
		max = defaultMaxWorkers
	}
	set := 0
	if deps.Lock != nil {
		set++
	}
	if deps.Stage2 != nil {
		set++
	}
	if deps.Merger != nil {
		set++
	}
	if set != 0 && set != 3 {
		panic("controller: integration requires all of Lock, Stage2, Merger (or none)")
	}
	if set == 3 && deps.Config.BaseBranch == "" {
		deps.Config.BaseBranch = "main"
	}
	return &Supervisor{
		deps:    deps,
		max:     max,
		workers: make(map[string]*Worker),
	}
}

// integrationConfigured reports whether Stage-2 promotion is wired.
func (s *Supervisor) integrationConfigured() bool {
	return s.deps.Lock != nil && s.deps.Stage2 != nil && s.deps.Merger != nil
}
```

- [ ] **Step 5: Extend `driveWorker` to promote past Stage1Passed**

In `supervisor.go`, find the Stage-1 tail of `driveWorker` (the final `switch` that sets `StateStage1Passed`). Replace that trailing `switch` block:

```go
	outcome, runErr := s.deps.Runner.RunStage1(ctx, snapshot)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case runErr != nil:
		s.transition(w, StateFailed, runErr.Error())
	case outcome.Passed:
		s.transition(w, StateStage1Passed, outcome.Detail)
	default:
		s.transition(w, StateFailed, outcome.Detail)
	}
```

with this version (note: the deferred unlock is removed here; locking is now
scoped per-transition so the long-running integration steps don't hold `s.mu`):

```go
	outcome, runErr := s.deps.Runner.RunStage1(ctx, snapshot)
	s.mu.Lock()
	switch {
	case runErr != nil:
		s.transition(w, StateFailed, runErr.Error())
	case outcome.Passed:
		s.transition(w, StateStage1Passed, outcome.Detail)
	default:
		s.transition(w, StateFailed, outcome.Detail)
	}
	passed := w.State == StateStage1Passed
	s.mu.Unlock()

	if passed && s.integrationConfigured() {
		s.integrate(ctx, id, sessionID)
	}
```

- [ ] **Step 6: Add the `integrate` method to `supervisor.go`**

```go
// integrate promotes a Stage1Passed worker serially: acquire the integration
// lock, run Stage 2, and on success merge to base and emit base_updated.
func (s *Supervisor) integrate(ctx context.Context, id, sessionID string) {
	release, err := s.deps.Lock.Acquire(ctx)
	if err != nil {
		s.mu.Lock()
		w := s.workers[id]
		s.transition(w, StateCancelled, "integration lock: "+err.Error())
		s.mu.Unlock()
		return
	}
	defer release()

	s.mu.Lock()
	w := s.workers[id]
	s.transition(w, StateIntegrating, "")
	snapshot := *w
	s.mu.Unlock()

	outcome, runErr := s.deps.Stage2.RunStage2(ctx, snapshot)
	if runErr != nil {
		s.mu.Lock()
		s.transition(w, StateFailed, runErr.Error())
		s.mu.Unlock()
		return
	}
	if !outcome.Passed {
		s.mu.Lock()
		s.transition(w, StateFailed, outcome.Detail)
		s.mu.Unlock()
		return
	}

	mergeRes, mergeErr := s.deps.Merger.MergeNoFF(ctx, sessionID, snapshot.Branch, defaultCallOpts())
	if mergeErr != nil || !mergeRes.Success {
		detail := "merge failed"
		if mergeErr != nil {
			detail = mergeErr.Error()
		} else if mergeRes.Output != "" {
			detail = mergeRes.Output
		}
		s.mu.Lock()
		s.transition(w, StateFailed, detail)
		s.mu.Unlock()
		return
	}

	sha := ""
	if shaRes, err := s.deps.Merger.CurrentCommitSHA(ctx, sessionID, defaultCallOpts()); err == nil && shaRes.Success {
		sha = strings.TrimSpace(shaRes.Output)
	}
	s.mu.Lock()
	s.transition(w, StateMerged, "merged")
	s.mu.Unlock()
	if s.deps.Bus != nil {
		s.deps.Bus.Publish(Event{Type: EventBaseUpdated, WorkerID: id, Payload: sha})
	}
}
```

- [ ] **Step 7: Add the `strings` import to `supervisor.go`**

Ensure the import block in `supervisor.go` includes `"strings"` (used by
`integrate` for `strings.TrimSpace`). The existing imports are `"context"`,
`"fmt"`, `"sync"`; add `"strings"`:

```go
import (
	"context"
	"fmt"
	"strings"
	"sync"
)
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/controller/ -run "TestIntegration|TestNewPanics" -v`
Expected: PASS (all seven tests).

- [ ] **Step 9: Run the full package + vet to confirm no regressions**

Run: `go test ./internal/controller/ && go vet ./internal/controller/`
Expected: PASS (slice-1 tests still green), no vet complaints.

- [ ] **Step 10: Commit**

```bash
git add internal/controller/supervisor.go internal/controller/integration_test.go
git commit -m "feat(controller): serial Stage-2 promotion drive with lock + merge + base_updated"
```

---

## Task 4: Smoke tests (real GuardedGit merge; Postgres lock gated on DATABASE_URL)

**Files:**
- Create: `internal/controller/integration_smoke_test.go`

- [ ] **Step 1: Write the real-`GuardedGit` merge smoke test**

```go
package controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/store"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestIntegrateWithRealGuardedGitMerge(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test requires real git; skipped under -short")
	}
	repo := t.TempDir()
	gitRun(t, repo, "init", "-b", "main")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "chore: base")

	// A feature branch with one extra commit, ready to merge into main.
	gitRun(t, repo, "checkout", "-b", "feature/x")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "feat: x")
	gitRun(t, repo, "checkout", "main")

	// Real DB + sessions row (policy_decisions FKs to sessions(id)).
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at)
		VALUES('smoke','n','open','.','','x','x')`); err != nil {
		t.Fatal(err)
	}

	// Real GuardedGit with an allow-git ruleset (DefaultRuleset would Ask on merge).
	raw := gitops.New(repo)
	rs := policy.Ruleset{Rules: []policy.Rule{
		{ID: "allow-git", Contains: "git", Effect: domain.EffectAllow, Reason: "test allows git ops"},
	}}
	guarded := policy.NewGuardedGit(policy.NewEngine(rs), policy.NewApprovals(), raw, repo, st.DB)

	// Drive integrate() directly via a one-worker supervisor whose Stage-1 passes
	// and whose worker branch is feature/x.
	bus := NewBus()
	defer bus.Close()
	s := New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{outcome: Outcome{Passed: true}},
		Bus:      bus,
		Config:   Config{MaxWorkers: 1, BaseBranch: "main"},
		Lock:     newFakeLock(),
		Stage2:   &fakeStage2{outcome: Outcome{Passed: true}},
		Merger:   guarded,
	})
	id, _ := s.Spawn(context.Background(), "smoke", WorkSpec{Name: "x", Branch: "feature/x"})
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Worker(id)
	if w.State != StateMerged {
		t.Fatalf("got %+v", w)
	}
	// feature.txt should now exist on main (merge succeeded).
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Fatalf("expected merged file on main: %v", err)
	}
}
```

- [ ] **Step 2: Add the Postgres-lock smoke test (gated on DATABASE_URL) in the same file**

```go
func TestPgLockSerializesWhenDatabaseURLSet(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL unset; skipping Postgres advisory-lock smoke")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	lock := NewPgLock(pool, 0x70746C6D, time.Minute) // arbitrary fixed key
	rel1, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Second acquire must block until rel1; prove with a short-ctx try that fails.
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	rel2, err := lock.Acquire(tctx)
	if err == nil {
		rel2()
		rel1()
		t.Fatal("expected second acquire to block while lock held")
	}

	rel1()
	// Now it should be acquirable.
	rel3, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel3()
}
```

- [ ] **Step 3: Run the smoke tests**

Run: `go test ./internal/controller/ -run "TestIntegrateWithRealGuardedGitMerge|TestPgLock" -v`
Expected: `TestIntegrateWithRealGuardedGitMerge` PASS; `TestPgLock...` PASS if `DATABASE_URL` is set, otherwise SKIP. Confirm the git smoke skips under `-short`:
`go test ./internal/controller/ -short -run TestIntegrateWithRealGuardedGitMerge -v` → SKIP.

- [ ] **Step 4: Resolve the `gitops.New` constructor if needed**

Before running, confirm `gitops.New(repo)` returns a value whose method set
satisfies `GitMerger` (it must expose `MergeNoFF` and `CurrentCommitSHA` with the
raw, unguarded signatures used by `RawGitOps` in `internal/policy/guard.go`). The
smoke wraps it in `policy.NewGuardedGit`, so the relevant signatures are the
guarded ones (`MergeNoFF(ctx, sessionID, branch, opts)` / `CurrentCommitSHA(ctx,
sessionID, opts)`), which `*policy.GuardedGit` already provides. If `gitops.New`
has a different name, read `internal/gitops/gitops.go` and use the actual
constructor — do not invent one.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/integration_smoke_test.go
git commit -m "test(controller): real GuardedGit merge smoke + Postgres lock smoke"
```

---

## Task 5: Architecture.md note + full suite

**Files:**
- Modify: `docs/Architecture.md`

- [ ] **Step 1: Append the package note**

Add this section to the end of `docs/Architecture.md`:

```markdown
## Controller Stage-2 integration (`internal/controller`)

Slice 2 of the orchestration layer adds serial promotion on top of the slice-1
supervisor. After a worker reaches `Stage1Passed`, `driveWorker` (when the
integration deps are configured) calls `integrate`: acquire a single
`IntegrationLock`, transition `Integrating`, run the injected `Stage2Runner`
against the real environment, and on success `MergeNoFF` the worker's branch to
base via a `GitMerger`, transition `Merged`, and publish `base_updated` with the
new base SHA. Stage-1 stays parallel (`MaxWorkers`); only the integration section
is serial, enforced by the lock. The production lock (`PgLock`) is a Postgres
session-level advisory lock — crash reclaim is automatic because the lock dies
with its connection — and needs no `Guarded*` wrapper, consistent with the
`internal/health` precedent for non-workspace Postgres access (no table is added;
the four-table schema is intact). The actual code change, the merge, *does* go
through `*policy.GuardedGit`. `New` panics if the integration deps are only
partially set (a wiring bug); all-nil preserves slice-1 behavior (stop at
`Stage1Passed`). The `base_updated` subscribers (propagation, relevance), the
current-with-base entry ticket, and the Stage-2-fail regression loop are deferred
to later slices, per the build order in `docs/ptolemy-architecture.html`.
```

- [ ] **Step 2: Run the full project test suite**

Run: `go test -p 1 ./...`
Expected: PASS across the module (controller included).

- [ ] **Step 3: Build all packages**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add docs/Architecture.md
git commit -m "docs: Architecture.md note for controller Stage-2 integration"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** lock interface + fake (Task 1), pg impl + stage2/merger seams
  (Task 2), supervisor drive + error handling + seriality + backward-compat +
  panic-on-partial (Task 3), real-git merge + Postgres lock smokes (Task 4),
  Architecture.md + full suite (Task 5). All spec sections mapped.
- **Locking discipline:** `integrate` never holds `s.mu` across the long-running
  `Acquire`/`RunStage2`/`MergeNoFF` calls — `s.mu` is taken only around each
  transition + snapshot read. This mirrors slice-1's per-transition locking and
  avoids serializing all workers behind one mutex.
- **`defaultCallOpts`** is reused from slice 1 (`runner.go`).
- **`collect`/`statesOf`/`equalStates`/`fakeWorktree`/`fakeRunner`** are reused
  from slice-1 `supervisor_test.go` (same package), so Task 3 tests must compile
  alongside them — they do, no redefinition.
- **Type consistency:** `IntegrationLock`, `Stage2Runner`, `GitMerger`, `Deps`
  fields (`Lock`/`Stage2`/`Merger`), `Config.BaseBranch`, `EventBaseUpdated`,
  `integrate`, `integrationConfigured` are used identically across tasks.
- **pgx import guard:** `pglock.go` references `pgx.ErrNoRows` and a
  `*pgxpool.Pool` compile-time check so both imports are used; if a reviewer
  prefers, drop the `pgx` import and the `var _ = pgx.ErrNoRows` line together.
