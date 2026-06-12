package main

import (
	"context"
	"database/sql"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/policy"
)

// idleCheckInterval is how often the idle-TTL loop checks for inactivity. The
// unload fires when idle time exceeds BrainIdleTTL; the check itself is a cheap
// ungated status read (no audit row per tick).
const idleCheckInterval = 30 * time.Second

// brainDeps carries the wired brain capability for main.go.
type brainDeps struct {
	guarded *policy.GuardedBrain // satisfies httpapi.BrainController (control router)
	waker   httpapi.Waker        // RAG auto-wake adapter; nil unless BrainAutoWake
}

// buildBrainDeps wires the guarded brain lifecycle skill, mirroring
// MaybeStartSweep/buildRAGDeps. Returns ok=false (logged) when the skill is
// disabled or misconfigured, so workerd keeps serving without it. When ok=true,
// the caller must run cleanup() on shutdown (stops the idle loop + the brain).
func buildBrainDeps(ctx context.Context, cfg config.Config, engine *policy.Engine, approvals *policy.Approvals, db *sql.DB) (deps brainDeps, cleanup func(), ok bool) {
	if !cfg.BrainControlEnabled {
		log.Info().Msg("brain skill disabled (set BRAIN_CONTROL_ENABLED=true)")
		return brainDeps{}, nil, false
	}
	if cfg.BrainModelsPath == "" {
		log.Warn().Msg("brain skill disabled: BRAIN_MODELS not set")
		return brainDeps{}, nil, false
	}
	reg, err := brain.LoadRegistry(cfg.BrainModelsPath)
	if err != nil {
		log.Warn().Err(err).Msg("brain skill disabled: model registry failed to load")
		return brainDeps{}, nil, false
	}
	if err := ensureBrainSession(ctx, db); err != nil {
		log.Warn().Err(err).Msg("brain skill disabled: could not ensure system session")
		return brainDeps{}, nil, false
	}

	host, port := brainHostPort(cfg.BrainBaseURL)
	mgr := brain.NewManager(reg, brain.NewExecLauncher(nil), brain.NewHTTPProbe(cfg.BrainBaseURL),
		host, port, cfg.BrainDefaultModel)
	gb := policy.NewGuardedBrain(engine, approvals, mgr, db)

	// Idle-TTL unloader: read status (ungated), unload (gated) only when idle.
	idleCtx, cancelIdle := context.WithCancel(ctx)
	idleDone := make(chan struct{})
	go func() {
		defer close(idleDone)
		brain.RunIdleLoop(idleCtx, idleCheckInterval, cfg.BrainIdleTTL, mgr.Status, func(c context.Context) error {
			return gb.Unload(c, httpapi.BrainSystemSession, policy.CallOpts{})
		})
	}()

	cleanup = func() {
		cancelIdle()
		select {
		case <-idleDone:
		case <-time.After(5 * time.Second):
			log.Error().Msg("brain idle loop did not stop within 5s")
		}
		_ = mgr.Stop(context.Background()) // workerd stops the brain it manages
	}

	deps = brainDeps{guarded: gb}
	if cfg.BrainAutoWake {
		deps.waker = &brainWaker{gb: gb}
	}
	log.Info().Str("default_model", cfg.BrainDefaultModel).Bool("auto_wake", cfg.BrainAutoWake).
		Msg("brain skill enabled")
	return deps, cleanup, true
}

// brainWaker adapts GuardedBrain.EnsureAwake (session+opts) to the bare
// httpapi.Waker the /chat hook expects.
type brainWaker struct{ gb *policy.GuardedBrain }

func (w *brainWaker) EnsureAwake(ctx context.Context) error {
	return w.gb.EnsureAwake(ctx, httpapi.BrainSystemSession, policy.CallOpts{})
}

// ensureBrainSession inserts the reserved system session used for brain audit
// rows (policy_decisions.session_id FK). Idempotent.
func ensureBrainSession(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO sessions(id,name,status,workspace,description,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?)`,
		httpapi.BrainSystemSession, "brain-system", "open", ".", "reserved session for brain audit", now, now)
	return err
}

// brainHostPort derives the llama-server bind host/port. The brain binds all
// interfaces (so co-located callers reach it); the port comes from BrainBaseURL.
func brainHostPort(baseURL string) (host, port string) {
	host = "0.0.0.0"
	port = "9000"
	if u, err := url.Parse(baseURL); err == nil && u.Port() != "" {
		port = u.Port()
	}
	return host, port
}
