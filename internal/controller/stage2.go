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
