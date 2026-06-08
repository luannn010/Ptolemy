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
	mu        sync.Mutex
	merged    []string
	mergeFail bool
	sha       string
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
