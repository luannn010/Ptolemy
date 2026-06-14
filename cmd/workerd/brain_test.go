package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/store"
)

func TestBuildBrainDeps_DisabledWhenFlagOff(t *testing.T) {
	deps, cleanup, ok := buildBrainDeps(context.Background(), config.Config{BrainControlEnabled: false}, nil, nil, nil)
	if ok {
		t.Fatal("expected ok=false when BRAIN_CONTROL_ENABLED off")
	}
	if cleanup != nil || deps.guarded != nil {
		t.Fatalf("expected empty deps when disabled: %+v", deps)
	}
}

func TestBuildBrainDeps_EnabledWiresAndEnsuresSession(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := config.Config{
		BrainControlEnabled: true,
		BrainLlamaBin:       "/bin/llama-server",
		BrainModelsDir:      t.TempDir(),
		BrainBaseURL:        "http://127.0.0.1:9000",
		BrainIdleTTL:        time.Minute,
		BrainAutoWake:       true,
	}
	deps, cleanup, ok := buildBrainDeps(context.Background(), cfg,
		policy.NewEngine(policy.DefaultRuleset()), policy.NewApprovals(), s.DB)
	if !ok {
		t.Fatal("expected ok=true when enabled")
	}
	if deps.guarded == nil || deps.waker == nil {
		t.Fatalf("expected guarded + waker wired, got %+v", deps)
	}
	t.Cleanup(cleanup)

	// the reserved system session must exist for the audit FK
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sessions WHERE id=?`, httpapi.BrainSystemSession).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected the brain-system session row, got %d", n)
	}
}
