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
	gate     chan struct{} // if non-nil, RunStage1 blocks until a token is received
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

// collect drains ch into a slice on a goroutine. The returned wait func blocks
// until the goroutine exits (after ch is closed), establishing a happens-before
// for safe reads of the slice.
func collect(ch <-chan Event) (*[]Event, func()) {
	evs := []Event{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			evs = append(evs, ev)
		}
	}()
	return &evs, func() { <-done }
}

func TestSupervisorHappyPathReachesStage1Passed(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe(64)
	defer unsub()
	evs, waitCollect := collect(ch)

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
	bus.Close()   // closes subscriber channel
	waitCollect() // wait for the collector goroutine to finish draining
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
	// release all four runs, then wait for completion
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
