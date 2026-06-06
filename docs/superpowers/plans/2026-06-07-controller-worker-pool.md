# Controller Worker-Pool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `internal/controller` — an in-process event bus, a worker lifecycle state machine, and a supervisor that drives N workers through an injected `Runner` with side effects routed only through `GuardedWorktree`.

**Architecture:** Three independently testable units in one package. The bus and state machine are pure (no side effects, no guard). The supervisor performs worktree side effects only through a small `WorktreeManager` consumer-interface that `*policy.GuardedWorktree` already satisfies, so unit tests inject an in-memory fake. No DB, no Docker, no model calls. Workers are driven only as far as `Stage1Passed` this slice.

**Tech Stack:** Go 1.25, stdlib (`context`, `sync`, `testing`), module `github.com/luannn010/ptolemy`. Tests are table-driven, fakes in `_test.go`, real-git smoke gated behind `!testing.Short()`.

**Spec:** `docs/superpowers/specs/2026-06-07-controller-worker-pool-design.md`

---

## File Structure

- Create `internal/controller/bus.go` — `Event`, `EventType`, `Bus` (Subscribe/Publish/Dropped/Close).
- Create `internal/controller/bus_test.go` — bus behaviour.
- Create `internal/controller/worker.go` — `State` enum, `Worker`, `WorkSpec`, `CanTransition`.
- Create `internal/controller/worker_test.go` — transition table.
- Create `internal/controller/runner.go` — `Runner` interface, `Outcome`, `WorktreeManager` interface.
- Create `internal/controller/supervisor.go` — `Config`, `Deps`, `Supervisor` (New/Spawn/Run/Worker/Workers/Shutdown).
- Create `internal/controller/supervisor_test.go` — supervisor behaviour with fakes.
- Create `internal/controller/supervisor_smoke_test.go` — real-git wiring smoke (gated `!testing.Short()`).
- Modify `docs/Architecture.md` — append a one-paragraph note.

Commit boundaries: (1) bus, (2) worker/state machine, (3) runner + supervisor, (4) smoke test, (5) Architecture.md.

---

## Task 1: Event bus

**Files:**
- Create: `internal/controller/bus.go`
- Test: `internal/controller/bus_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package controller

import (
	"sync"
	"testing"
)

func TestBusFanOutToAllSubscribers(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch1, unsub1 := b.Subscribe(4)
	defer unsub1()
	ch2, unsub2 := b.Subscribe(4)
	defer unsub2()

	b.Publish(Event{Type: EventWorkerStateChanged, WorkerID: "w1", From: StatePending, To: StateProvisioning})

	for i, ch := range []<-chan Event{ch1, ch2} {
		ev := <-ch
		if ev.WorkerID != "w1" || ev.To != StateProvisioning {
			t.Fatalf("subscriber %d got %+v", i, ev)
		}
	}
}

func TestBusUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, unsub := b.Subscribe(4)
	unsub()
	unsub() // idempotent, must not panic

	b.Publish(Event{Type: EventWorkerStateChanged, WorkerID: "w1"})

	if _, ok := <-ch; ok {
		t.Fatal("expected channel closed after unsubscribe")
	}
}

func TestBusSlowSubscriberDropsWithoutBlocking(t *testing.T) {
	b := NewBus()
	defer b.Close()

	// buffer 1, never drained
	_, unsub := b.Subscribe(1)
	defer unsub()

	for i := 0; i < 10; i++ {
		b.Publish(Event{Type: EventWorkerStateChanged, WorkerID: "w1"})
	}

	if b.Dropped() == 0 {
		t.Fatal("expected dropped events > 0")
	}
}

func TestBusCloseIsIdempotentAndPublishAfterCloseIsNoop(t *testing.T) {
	b := NewBus()
	ch, _ := b.Subscribe(1)
	b.Close()
	b.Close() // idempotent

	if _, ok := <-ch; ok {
		t.Fatal("expected subscriber channel closed on bus Close")
	}
	b.Publish(Event{Type: EventWorkerStateChanged}) // must not panic
}

func TestBusConcurrentPublishIsSafe(t *testing.T) {
	b := NewBus()
	defer b.Close()
	ch, unsub := b.Subscribe(1000)
	defer unsub()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.Publish(Event{Type: EventWorkerStateChanged}) }()
	}
	wg.Wait()
	_ = ch
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/controller/ -run TestBus -v`
Expected: FAIL — `undefined: NewBus`, `undefined: Event`, etc.

- [ ] **Step 3: Write the minimal implementation**

```go
package controller

import "sync"

// EventType identifies a controller event.
type EventType string

const (
	// EventWorkerStateChanged is published on every legal worker state transition.
	EventWorkerStateChanged EventType = "worker.state_changed"
	// EventBaseUpdated is reserved for a later propagation slice.
	EventBaseUpdated EventType = "base_updated"
)

// Event is a single message on the bus.
type Event struct {
	Type     EventType
	WorkerID string
	From     State
	To       State
	Payload  any
}

type subscriber struct {
	ch chan Event
}

// Bus is an in-process pub/sub bus with non-blocking fan-out. Each subscriber
// owns a bounded buffered channel; a full buffer drops the event (counted)
// rather than blocking the publisher.
type Bus struct {
	mu      sync.Mutex
	subs    map[int]*subscriber
	nextID  int
	dropped int
	closed  bool
}

// NewBus returns an open bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[int]*subscriber)}
}

// Subscribe registers a subscriber with the given channel buffer and returns the
// receive channel plus an idempotent unsubscribe func.
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}

	id := b.nextID
	b.nextID++
	s := &subscriber{ch: make(chan Event, buffer)}
	b.subs[id] = s

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if sub, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(sub.ch)
			}
		})
	}
	return s.ch, unsub
}

// Publish delivers ev to every subscriber without blocking the caller.
func (b *Bus) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, s := range b.subs {
		select {
		case s.ch <- ev:
		default:
			b.dropped++
		}
	}
}

// Dropped returns the total number of events dropped due to full buffers.
func (b *Bus) Dropped() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// Close closes all subscriber channels. Idempotent; Publish after Close is a no-op.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, s := range b.subs {
		close(s.ch)
		delete(b.subs, id)
	}
}
```

Note: `State` is defined in Task 2 (`worker.go`); the package won't compile in isolation until Task 2 lands. That's fine — both are in the same package and the bus test only references `State` constants defined there. If running Task 1 tests standalone before Task 2, temporarily inline `type State string` — but the recommended flow is to complete Task 2's `worker.go` first if the compiler complains. To keep Task 1 self-contained, **define `State` and its constants in `worker.go` (Task 2) before running these tests**, OR reorder: do Task 2 first. The plan orders bus first for commit logic; if the compiler blocks, jump to Task 2 Step 3, then return.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/controller/ -run TestBus -v`
Expected: PASS (after `State` exists — see note; if blocked, complete Task 2 Step 3 first).

- [ ] **Step 5: Commit**

```bash
git add internal/controller/bus.go internal/controller/bus_test.go
git commit -m "feat(controller): in-process event bus with non-blocking fan-out"
```

---

## Task 2: Worker lifecycle state machine

**Files:**
- Create: `internal/controller/worker.go`
- Test: `internal/controller/worker_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package controller

import "testing"

func TestCanTransitionLegal(t *testing.T) {
	legal := []struct{ from, to State }{
		{StatePending, StateProvisioning},
		{StateProvisioning, StateRunning},
		{StateRunning, StateStage1Passed},
		{StateStage1Passed, StateIntegrating},
		{StateIntegrating, StateMerged},
		{StatePending, StateFailed},
		{StateProvisioning, StateFailed},
		{StateRunning, StateFailed},
		{StatePending, StateCancelled},
		{StateRunning, StateCancelled},
		{StateStage1Passed, StateCancelled},
	}
	for _, c := range legal {
		if !CanTransition(c.from, c.to) {
			t.Errorf("expected %s->%s legal", c.from, c.to)
		}
	}
}

func TestCanTransitionIllegal(t *testing.T) {
	illegal := []struct{ from, to State }{
		{StatePending, StateRunning},      // must provision first
		{StateProvisioning, StateMerged},  // skips stages
		{StateMerged, StateRunning},       // terminal
		{StateFailed, StateRunning},       // terminal
		{StateCancelled, StatePending},    // terminal
		{StateRunning, StatePending},      // no going back
		{StateStage1Passed, StateRunning}, // no going back
	}
	for _, c := range illegal {
		if CanTransition(c.from, c.to) {
			t.Errorf("expected %s->%s illegal", c.from, c.to)
		}
	}
}

func TestTerminalStatesHaveNoOutgoing(t *testing.T) {
	all := []State{
		StatePending, StateProvisioning, StateRunning, StateStage1Passed,
		StateIntegrating, StateMerged, StateFailed, StateCancelled,
	}
	for _, term := range []State{StateMerged, StateFailed, StateCancelled} {
		for _, to := range all {
			if CanTransition(term, to) {
				t.Errorf("terminal %s should not transition to %s", term, to)
			}
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/controller/ -run TestCanTransition -v`
Expected: FAIL — `undefined: CanTransition` / `undefined: StatePending`.

- [ ] **Step 3: Write the minimal implementation**

```go
package controller

// State is a worker lifecycle state.
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

// WorkSpec describes a unit of work assigned to a worker.
type WorkSpec struct {
	Name   string // worker / worktree name
	Branch string // optional; worktree.Manager defaults when empty
}

// Worker is the controller's record for one sandboxed unit of work.
type Worker struct {
	ID       string
	Spec     WorkSpec
	Worktree string // filesystem path, set after provisioning
	Branch   string
	State    State
	Detail   string // last outcome or error detail

	sessionID string // unexported: policy session that spawned this worker
}

// transitions maps each state to the set of states it may legally move to.
// Merged, Failed, Cancelled are terminal (absent / empty here).
var transitions = map[State][]State{
	StatePending:      {StateProvisioning, StateFailed, StateCancelled},
	StateProvisioning: {StateRunning, StateFailed, StateCancelled},
	StateRunning:      {StateStage1Passed, StateFailed, StateCancelled},
	StateStage1Passed: {StateIntegrating, StateFailed, StateCancelled},
	StateIntegrating:  {StateMerged, StateFailed, StateCancelled},
}

// CanTransition reports whether from->to is a legal lifecycle transition.
func CanTransition(from, to State) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/controller/ -run "TestCanTransition|TestTerminal" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/worker.go internal/controller/worker_test.go
git commit -m "feat(controller): worker lifecycle state machine with transition table"
```

---

## Task 3: Runner interface + Supervisor

**Files:**
- Create: `internal/controller/runner.go`
- Create: `internal/controller/supervisor.go`
- Test: `internal/controller/supervisor_test.go`

- [ ] **Step 1: Write `runner.go` (interfaces + value types — no test of its own)**

```go
package controller

import (
	"context"

	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/worktree"
)

// Outcome is the result of a Stage-1 run. Passed=false is a clean red;
// a non-nil error from the Runner is an execution fault. Both are distinguished
// by Detail and both move the worker to Failed in this slice.
type Outcome struct {
	Passed bool
	Detail string
}

// Runner drives a worker's Stage-1 work. The real model-backed implementation
// lands in a later slice; tests inject a fake.
type Runner interface {
	RunStage1(ctx context.Context, w Worker) (Outcome, error)
}

// WorktreeManager is the consumer-side interface the supervisor depends on.
// *policy.GuardedWorktree satisfies it as-is (its method set is a superset),
// so production injects the guarded type and tests inject an in-memory fake.
// No raw adapter is ever reachable from the supervisor.
type WorktreeManager interface {
	Create(ctx context.Context, sessionID, name, branch string, opts policy.CallOpts) (worktree.Result, error)
	Remove(ctx context.Context, sessionID, name string, opts policy.CallOpts) (worktree.Result, error)
}
```

- [ ] **Step 2: Write the failing supervisor tests**

```go
package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/worktree"
)

// fakeWorktree records calls and returns successful results unless failCreate set.
type fakeWorktree struct {
	mu         sync.Mutex
	created    []string
	removed    []string
	failCreate bool
}

func (f *fakeWorktree) Create(ctx context.Context, sessionID, name, branch string, opts policy.CallOpts) (worktree.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreate {
		return worktree.Result{Success: false, Output: "boom"}, nil
	}
	f.created = append(f.created, name)
	return worktree.Result{Success: true, Worktree: "/tmp/" + name, Branch: branch}, nil
}

func (f *fakeWorktree) Remove(ctx context.Context, sessionID, name string, opts policy.CallOpts) (worktree.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, name)
	return worktree.Result{Success: true}, nil
}

// fakeRunner returns a fixed outcome/error and optionally tracks max concurrency.
type fakeRunner struct {
	outcome  Outcome
	err      error
	inFlight int32
	maxSeen  int32
	gate     chan struct{} // if non-nil, RunStage1 blocks until closed
}

func (r *fakeRunner) RunStage1(ctx context.Context, w Worker) (Outcome, error) {
	n := atomic.AddInt32(&r.inFlight, 1)
	for {
		old := atomic.LoadInt32(&r.maxSeen)
		if n <= old || atomic.CompareAndSwapInt32(&r.maxSeen, old, n) {
			break
		}
	}
	if r.gate != nil {
		<-r.gate
	}
	atomic.AddInt32(&r.inFlight, -1)
	return r.outcome, r.err
}

func collect(ch <-chan Event) *[]Event {
	var mu sync.Mutex
	evs := []Event{}
	go func() {
		for ev := range ch {
			mu.Lock()
			evs = append(evs, ev)
			mu.Unlock()
		}
	}()
	return &evs
}

func TestSupervisorHappyPathReachesStage1Passed(t *testing.T) {
	bus := NewBus()
	defer bus.Close()
	ch, unsub := bus.Subscribe(64)
	defer unsub()
	evs := collect(ch)

	wt := &fakeWorktree{}
	s := New(Deps{
		Worktree: wt,
		Runner:   &fakeRunner{outcome: Outcome{Passed: true, Detail: "green"}},
		Bus:      bus,
		Config:   Config{MaxWorkers: 2},
	})

	id, err := s.Spawn(context.Background(), "sess", WorkSpec{Name: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	w, ok := s.Worker(id)
	if !ok || w.State != StateStage1Passed {
		t.Fatalf("got %+v ok=%v", w, ok)
	}
	if len(wt.created) != 1 || wt.created[0] != "alpha" {
		t.Fatalf("expected worktree created for alpha, got %v", wt.created)
	}
	bus.Close() // flush collector
	want := []State{StateProvisioning, StateRunning, StateStage1Passed}
	if got := statesOf(*evs, id); !equalStates(got, want) {
		t.Fatalf("event states = %v, want %v", got, want)
	}
}

func TestSupervisorCleanRedGoesToFailed(t *testing.T) {
	s := New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{outcome: Outcome{Passed: false, Detail: "tests red"}},
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 1},
	})
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "beta"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateFailed || w.Detail != "tests red" {
		t.Fatalf("got %+v", w)
	}
}

func TestSupervisorRunnerErrorGoesToFailedWithErrorDetail(t *testing.T) {
	s := New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{err: context.DeadlineExceeded},
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 1},
	})
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "gamma"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateFailed || w.Detail == "tests red" || w.Detail == "" {
		t.Fatalf("expected failed with error detail, got %+v", w)
	}
}

func TestSupervisorWorktreeCreateFailureIsolatesWorker(t *testing.T) {
	s := New(Deps{
		Worktree: &fakeWorktree{failCreate: true},
		Runner:   &fakeRunner{outcome: Outcome{Passed: true}},
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 1},
	})
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "delta"})
	_ = s.Run(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateFailed {
		t.Fatalf("expected Failed on worktree create failure, got %+v", w)
	}
}

func TestSupervisorRespectsMaxWorkers(t *testing.T) {
	gate := make(chan struct{})
	r := &fakeRunner{outcome: Outcome{Passed: true}, gate: gate}
	s := New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   r,
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 2},
	})
	for _, n := range []string{"a", "b", "c", "d"} {
		if _, err := s.Spawn(context.Background(), "sess", WorkSpec{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	go func() { _ = s.Run(context.Background()); close(done) }()
	// release everything, then wait for completion
	for i := 0; i < 4; i++ {
		gate <- struct{}{}
	}
	close(gate)
	<-done
	if max := atomic.LoadInt32(&r.maxSeen); max > 2 {
		t.Fatalf("max concurrent = %d, want <= 2", max)
	}
}

func TestSupervisorShutdownCancelsAndRemovesWorktrees(t *testing.T) {
	wt := &fakeWorktree{}
	s := New(Deps{
		Worktree: wt,
		Runner:   &fakeRunner{outcome: Outcome{Passed: true}},
		Bus:      NewBus(),
		Config:   Config{MaxWorkers: 2},
	})
	id, _ := s.Spawn(context.Background(), "sess", WorkSpec{Name: "epsilon"})
	_ = s.Run(context.Background())
	s.Shutdown(context.Background())
	w, _ := s.Worker(id)
	if w.State != StateCancelled && w.State != StateStage1Passed {
		t.Fatalf("unexpected post-shutdown state %+v", w)
	}
	if len(wt.removed) == 0 {
		t.Fatal("expected worktree removed on shutdown")
	}
}

// helpers
func statesOf(evs []Event, id string) []State {
	var out []State
	for _, e := range evs {
		if e.Type == EventWorkerStateChanged && e.WorkerID == id {
			out = append(out, e.To)
		}
	}
	return out
}

func equalStates(a, b []State) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/controller/ -run TestSupervisor -v`
Expected: FAIL — `undefined: New`, `undefined: Deps`, `undefined: Config`.

- [ ] **Step 4: Write `supervisor.go`**

```go
package controller

import (
	"context"
	"fmt"
	"sync"
)

const defaultMaxWorkers = 4

// Config tunes the supervisor.
type Config struct {
	MaxWorkers int // concurrency bound; defaults to 4 when <= 0
}

// Deps are the supervisor's injected dependencies.
type Deps struct {
	Worktree WorktreeManager
	Runner   Runner
	Bus      *Bus
	Config   Config
}

// Supervisor owns the in-memory worker registry and drives workers through
// their lifecycle. All side effects flow through the injected WorktreeManager.
type Supervisor struct {
	deps    Deps
	max     int
	mu      sync.Mutex
	workers map[string]*Worker
	order   []string
	nextID  int
}

// New constructs a Supervisor.
func New(deps Deps) *Supervisor {
	max := deps.Config.MaxWorkers
	if max <= 0 {
		max = defaultMaxWorkers
	}
	return &Supervisor{
		deps:    deps,
		max:     max,
		workers: make(map[string]*Worker),
	}
}

// Spawn registers a new Pending worker and returns its id.
func (s *Supervisor) Spawn(ctx context.Context, sessionID string, spec WorkSpec) (string, error) {
	if spec.Name == "" {
		return "", fmt.Errorf("controller: WorkSpec.Name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("w%d", s.nextID)
	s.nextID++
	s.workers[id] = &Worker{ID: id, Spec: spec, Branch: spec.Branch, State: StatePending, sessionID: sessionID}
	s.order = append(s.order, id)
	return id, nil
}

// transition applies from->to if legal, records detail, and publishes an event.
// Returns false if the transition is illegal (no mutation, no event).
func (s *Supervisor) transition(w *Worker, to State, detail string) bool {
	if !CanTransition(w.State, to) {
		return false
	}
	from := w.State
	w.State = to
	w.Detail = detail
	if s.deps.Bus != nil {
		s.deps.Bus.Publish(Event{Type: EventWorkerStateChanged, WorkerID: w.ID, From: from, To: to})
	}
	return true
}

// Run drives every spawned, non-terminal worker concurrently up to MaxWorkers,
// returning when all have reached Stage1Passed, Failed, or Cancelled.
func (s *Supervisor) Run(ctx context.Context) error {
	s.mu.Lock()
	ids := append([]string(nil), s.order...)
	s.mu.Unlock()

	sem := make(chan struct{}, s.max)
	var wg sync.WaitGroup
	for _, id := range ids {
		s.mu.Lock()
		w := s.workers[id]
		pending := w.State == StatePending
		sessionID := w.sessionID
		s.mu.Unlock()
		if !pending {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id, sessionID string) {
			defer wg.Done()
			defer func() { <-sem }()
			s.driveWorker(ctx, id, sessionID)
		}(id, sessionID)
	}
	wg.Wait()
	return nil
}

// driveWorker provisions a worktree then runs Stage 1 for one worker.
func (s *Supervisor) driveWorker(ctx context.Context, id, sessionID string) {
	s.mu.Lock()
	w := s.workers[id]
	s.mu.Unlock()

	if ctx.Err() != nil {
		s.mu.Lock()
		s.transition(w, StateCancelled, "cancelled before provisioning")
		s.mu.Unlock()
		return
	}

	// Provision
	s.mu.Lock()
	s.transition(w, StateProvisioning, "")
	s.mu.Unlock()
	res, err := s.deps.Worktree.Create(ctx, sessionID, w.Spec.Name, w.Spec.Branch, defaultCallOpts())
	if err != nil || !res.Success {
		detail := "worktree create failed"
		if err != nil {
			detail = err.Error()
		} else if res.Output != "" {
			detail = res.Output
		}
		s.mu.Lock()
		s.transition(w, StateFailed, detail)
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	w.Worktree = res.Worktree
	if res.Branch != "" {
		w.Branch = res.Branch
	}
	snapshot := *w
	s.mu.Unlock()

	// Run Stage 1
	s.mu.Lock()
	s.transition(w, StateRunning, "")
	s.mu.Unlock()
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
}

// Worker returns a copy of the worker with the given id.
func (s *Supervisor) Worker(id string) (Worker, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workers[id]
	if !ok {
		return Worker{}, false
	}
	return *w, true
}

// Workers returns copies of all workers in spawn order.
func (s *Supervisor) Workers() []Worker {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Worker, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, *s.workers[id])
	}
	return out
}

// Shutdown cancels in-flight/non-terminal workers and removes their worktrees.
func (s *Supervisor) Shutdown(ctx context.Context) {
	s.mu.Lock()
	type rm struct{ name string }
	var toRemove []rm
	for _, id := range s.order {
		w := s.workers[id]
		if w.Worktree != "" {
			toRemove = append(toRemove, rm{name: w.Spec.Name})
		}
		if CanTransition(w.State, StateCancelled) {
			s.transition(w, StateCancelled, "shutdown")
		}
	}
	s.mu.Unlock()
	for _, r := range toRemove {
		_, _ = s.deps.Worktree.Remove(ctx, "shutdown", r.name, defaultCallOpts())
	}
}
```

- [ ] **Step 5: Add `defaultCallOpts` helper to `runner.go`**

Append to `internal/controller/runner.go`:

```go
// defaultCallOpts returns the call options the supervisor passes to guarded
// adapters. Empty for now: low-risk worktree create/remove run under policy
// allow rules; high-risk gating is the policy engine's concern, not the
// supervisor's.
func defaultCallOpts() policy.CallOpts { return policy.CallOpts{} }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/controller/ -run TestSupervisor -v`
Expected: PASS (all six supervisor tests).

- [ ] **Step 7: Run the full package and vet**

Run: `go test ./internal/controller/ -v && go vet ./internal/controller/`
Expected: PASS, no vet complaints.

- [ ] **Step 8: Commit**

```bash
git add internal/controller/runner.go internal/controller/supervisor.go internal/controller/supervisor_test.go
git commit -m "feat(controller): supervisor drives workers through Runner via GuardedWorktree"
```

---

## Task 4: Real-git wiring smoke test

**Files:**
- Create: `internal/controller/supervisor_smoke_test.go`

- [ ] **Step 1: Write the smoke test (gated behind !testing.Short())**

```go
package controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/worktree"
)

func setupRepoForSmoke(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "chore: initial commit")
	return dir
}

func TestSupervisorWithRealGuardedWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test requires real git; skipped under -short")
	}
	repo := setupRepoForSmoke(t)
	wtDir := filepath.Join(repo, ".worktrees")

	// Real worktree manager wrapped by the real guard, with an allow-all engine.
	mgr := worktree.NewManager(repo, wtDir)
	engine := policy.NewAllowAllEngine() // see note below
	approvals := policy.NewApprovals()
	guarded := policy.NewGuardedWorktree(engine, approvals, mgr, repo, nil)

	bus := NewBus()
	defer bus.Close()
	s := New(Deps{
		Worktree: guarded,
		Runner:   &fakeRunner{outcome: Outcome{Passed: true, Detail: "green"}},
		Bus:      bus,
		Config:   Config{MaxWorkers: 2},
	})

	id, err := s.Spawn(context.Background(), "smoke", WorkSpec{Name: "smoke-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Worker(id)
	if w.State != StateStage1Passed {
		t.Fatalf("got %+v", w)
	}
	if _, err := os.Stat(w.Worktree); err != nil {
		t.Fatalf("expected worktree dir on disk: %v", err)
	}
	s.Shutdown(context.Background())
}
```

- [ ] **Step 2: Resolve the engine/approvals constructors**

Before running, confirm the real constructor names by reading `internal/policy/engine.go` and `internal/policy/approve.go`. The test above assumes `policy.NewAllowAllEngine()` and `policy.NewApprovals()`. If those exact constructors don't exist:
- Use whatever constructor builds an `*Engine` (e.g. `policy.NewEngine(rules)` with an allow-all rule set, or load `.ptolemy/policy.json`), and
- Use the real approvals constructor.
Adjust the two lines accordingly. Do NOT invent a constructor — match what exists. Pass `nil` for the `*sql.DB` only if `guardCore.recordDecision` tolerates a nil DB; if it panics on nil, open an in-memory sqlite the way other policy tests do (read `internal/policy/guard_test.go` for the established pattern) and use that.

- [ ] **Step 3: Run the smoke test**

Run: `go test ./internal/controller/ -run TestSupervisorWithRealGuardedWorktree -v`
Expected: PASS. Then confirm it's skipped under short: `go test ./internal/controller/ -short -run TestSupervisorWithRealGuardedWorktree -v` → SKIP.

- [ ] **Step 4: Commit**

```bash
git add internal/controller/supervisor_smoke_test.go
git commit -m "test(controller): real GuardedWorktree wiring smoke test"
```

---

## Task 5: Architecture.md note + full suite

**Files:**
- Modify: `docs/Architecture.md`

- [ ] **Step 1: Append the package note**

Add this section to the end of `docs/Architecture.md`:

```markdown
## Controller worker-pool (`internal/controller`)

The first deterministic slice of the multi-agent orchestration layer. Three pure
units plus a supervisor: an in-process `Bus` (non-blocking fan-out — each
subscriber drains its own bounded channel; a full buffer drops + counts rather
than stalling the publisher), a worker lifecycle `State` machine (`Pending →
Provisioning → Running → Stage1Passed → Integrating → Merged`, with `Failed` /
`Cancelled` reachable from every non-terminal state; `CanTransition` validates
against a transition table), and a `Supervisor` that spawns N workers into git
worktrees and drives each through an injected `Runner` up to `Stage1Passed`. The
supervisor performs side effects only through a `WorktreeManager` consumer
interface satisfied by `*policy.GuardedWorktree`, so it never touches a raw
adapter — consistent with the harness rule. The registry is in-memory (no schema
change); Docker, mocks, the integration lock/Stage 2, `base_updated`
propagation, and wiring in the model are deferred to later slices, per the build
order in `docs/ptolemy-architecture.html` ("wire in the model last"). The
`Runner` interface is the seam the model-backed implementation plugs into later.
```

- [ ] **Step 2: Run the full project test suite**

Run: `make test`
Expected: PASS across the module (controller package included), per the `-p 1` convention.

- [ ] **Step 3: Build all binaries**

Run: `make build`
Expected: builds `workerd`, `ptolemy-mcp`, `ptolemy`, `ptolemy-memory` with no errors.

- [ ] **Step 4: Commit**

```bash
git add docs/Architecture.md
git commit -m "docs: Architecture.md note for internal/controller worker-pool"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** bus (Task 1), state machine (Task 2), Runner + supervisor + error handling + MaxWorkers + Shutdown (Task 3), real-guard wiring (Task 4), Architecture.md + DoD (Task 5). All spec sections mapped.
- **sessionID storage:** stored in an unexported `Worker.sessionID` field set by `Spawn` and read by `Run`. Unexported, so it never leaks through the public `Worker` snapshot or JSON; `driveWorker` passes it to `GuardedWorktree.Create`/`Remove`.
- **nil DB in smoke test:** flagged in Task 4 Step 2 — verify against `internal/policy/guard_test.go` rather than assuming.
- **Type consistency:** `State`, `Worker`, `Outcome`, `Runner`, `WorktreeManager`, `Deps`, `Config`, `Bus`, `Event` names are used identically across all tasks.
```
