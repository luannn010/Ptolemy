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

// defaultCallOpts returns the call options the supervisor passes to guarded
// adapters. Empty for now: low-risk worktree create/remove run under policy
// allow rules; high-risk gating is the policy engine's concern, not the
// supervisor's.
func defaultCallOpts() policy.CallOpts { return policy.CallOpts{} }
