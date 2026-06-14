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
	ResolveSpec(spec brain.Spec) brain.Spec
	Load(ctx context.Context, spec brain.Spec) error
	Resume(ctx context.Context) error
	EnsureAwake(ctx context.Context) error
	Hibernate(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() brain.Status
	ListModels() ([]brain.DiscoveredModel, error)
}

// GuardedBrain gates every brain action through the policy engine and audits it
// to policy_decisions, mirroring GuardedRunner/GuardedGit. load carries the full
// resolved spec (ask/OOB); resume/wake/hibernate/status/models carry no spec
// (allow); stop carries no spec but is kept ask as a manual-teardown gate.
type GuardedBrain struct {
	core guardCore
	raw  RawBrain
}

func NewGuardedBrain(engine *Engine, approvals *Approvals, raw RawBrain, db *sql.DB) *GuardedBrain {
	return &GuardedBrain{core: guardCore{engine: engine, approvals: approvals, db: db}, raw: raw}
}

func (g *GuardedBrain) intent(action string, args ...string) domain.Intent {
	return domain.Intent{Kind: "brain." + action, Program: "brain", Args: append([]string{action}, args...)}
}

// Load gates on "brain load <argv…>" (ask/OOB). The resolved argv is in the
// intent so the hash and the deny rules cover the whole command — a destructive
// token in any spec field is denied, and approving one spec can't authorize
// another.
func (g *GuardedBrain) Load(ctx context.Context, sessionID string, spec brain.Spec, opts CallOpts) error {
	resolved := g.raw.ResolveSpec(spec)
	intent := g.intent("load", resolved.Argv()...)
	intent.Targets = []string{resolved.GGUF}
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return err
	}
	return g.raw.Load(ctx, resolved)
}

// Resume gates on "brain resume" (allow) — relaunch the already-approved spec.
func (g *GuardedBrain) Resume(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("resume"), opts); err != nil {
		return err
	}
	return g.raw.Resume(ctx)
}

// EnsureAwake gates on "brain wake" (allow) — the /chat auto-wake hook.
func (g *GuardedBrain) EnsureAwake(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("wake"), opts); err != nil {
		return err
	}
	return g.raw.EnsureAwake(ctx)
}

// Hibernate gates on "brain hibernate" (allow) — frees VRAM, keeps the spec.
func (g *GuardedBrain) Hibernate(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("hibernate"), opts); err != nil {
		return err
	}
	return g.raw.Hibernate(ctx)
}

// Stop gates on "brain stop" (ask/OOB) — manual full teardown.
func (g *GuardedBrain) Stop(ctx context.Context, sessionID string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.intent("stop"), opts); err != nil {
		return err
	}
	return g.raw.Stop(ctx)
}

// Status gates on "brain status" (allow) and returns a snapshot.
func (g *GuardedBrain) Status(ctx context.Context, sessionID string, opts CallOpts) (brain.Status, error) {
	if err := g.core.gate(ctx, sessionID, g.intent("status"), opts); err != nil {
		return brain.Status{}, err
	}
	return g.raw.Status(), nil
}

// ListModels gates on "brain models" (allow) and returns discovered ggufs.
func (g *GuardedBrain) ListModels(ctx context.Context, sessionID string, opts CallOpts) ([]brain.DiscoveredModel, error) {
	if err := g.core.gate(ctx, sessionID, g.intent("models"), opts); err != nil {
		return nil, err
	}
	return g.raw.ListModels()
}
