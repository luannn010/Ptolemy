package policy

import (
	"context"
	"database/sql"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/domain"
)

// RawBrain is the unguarded brain lifecycle mechanism (internal/brain.Manager).
// Reaching it only via GuardedBrain keeps process spawn/kill behind the policy
// harness, like every other side-effecting adapter.
type RawBrain interface {
	EnsureAwake(ctx context.Context) error
	Wake(ctx context.Context, model string) error
	Switch(ctx context.Context, model string) error
	Stop(ctx context.Context) error
	Unload(ctx context.Context) error
	Status() brain.Status
}

// GuardedBrain gates every brain action through the policy engine and audits it
// to policy_decisions, mirroring GuardedRunner/GuardedGit. Automatic actions
// (wake/autounload/status) are policy-allow by convention so they don't block;
// manual destructive actions (switch/stop) are policy-ask (OOB).
type GuardedBrain struct {
	core guardCore
	raw  RawBrain
}

func NewGuardedBrain(engine *Engine, approvals *Approvals, raw RawBrain, db *sql.DB) *GuardedBrain {
	return &GuardedBrain{core: guardCore{engine: engine, approvals: approvals, db: db}, raw: raw}
}

func (g *GuardedBrain) brainIntent(action string, args ...string) domain.Intent {
	return domain.Intent{
		Kind:    "brain." + action,
		Program: "brain",
		Args:    append([]string{action}, args...),
	}
}

// EnsureAwake and Wake both gate on the "brain wake" intent (allow) — the
// auto-wake hook and manual start share one rule.
func (g *GuardedBrain) EnsureAwake(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.brainIntent("wake"), opts); err != nil {
		return err
	}
	return g.raw.EnsureAwake(ctx)
}

func (g *GuardedBrain) Wake(ctx context.Context, sessionID, model string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.brainIntent("wake", model), opts); err != nil {
		return err
	}
	return g.raw.Wake(ctx, model)
}

// Unload gates on the "brain autounload" intent (allow) — the idle-TTL path.
func (g *GuardedBrain) Unload(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.brainIntent("autounload"), opts); err != nil {
		return err
	}
	return g.raw.Unload(ctx)
}

// Stop gates on "brain stop" (ask/OOB) — manual, destructive.
func (g *GuardedBrain) Stop(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.brainIntent("stop"), opts); err != nil {
		return err
	}
	return g.raw.Stop(ctx)
}

// Switch gates on "brain switch" (ask/OOB) — manual model swap.
func (g *GuardedBrain) Switch(ctx context.Context, sessionID, model string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.brainIntent("switch", model), opts); err != nil {
		return err
	}
	return g.raw.Switch(ctx, model)
}

// Status gates on "brain status" (allow) and returns a snapshot.
func (g *GuardedBrain) Status(ctx context.Context, sessionID string, opts CallOpts) (brain.Status, error) {
	if err := g.core.gate(ctx, sessionID, g.brainIntent("status"), opts); err != nil {
		return brain.Status{}, err
	}
	return g.raw.Status(), nil
}
