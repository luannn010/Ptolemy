package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const defaultMaxWorkers = 4

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
// Callers must hold s.mu.
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
}

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
	type rm struct{ name, sessionID string }
	var toRemove []rm
	for _, id := range s.order {
		w := s.workers[id]
		if w.Worktree != "" {
			toRemove = append(toRemove, rm{name: w.Spec.Name, sessionID: w.sessionID})
		}
		if CanTransition(w.State, StateCancelled) {
			s.transition(w, StateCancelled, "shutdown")
		}
	}
	s.mu.Unlock()
	for _, r := range toRemove {
		_, _ = s.deps.Worktree.Remove(ctx, r.sessionID, r.name, defaultCallOpts())
	}
}
