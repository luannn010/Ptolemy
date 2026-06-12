package main

import (
	"context"
	"os"
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

func TestBuildBrainDeps_DisabledWhenNoModelsPath(t *testing.T) {
	cfg := config.Config{BrainControlEnabled: true, BrainModelsPath: ""}
	_, _, ok := buildBrainDeps(context.Background(), cfg, nil, nil, nil)
	if ok {
		t.Fatal("expected ok=false when BRAIN_MODELS unset")
	}
}

func TestBuildBrainDeps_EnabledWiresAndEnsuresSession(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(modelsPath, []byte(
		`{"models":[{"name":"qwen9b","binary":"/bin/llama-server","gguf":"/m/q.gguf","args":["--ctx-size","32768"]}]}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := config.Config{
		BrainControlEnabled: true,
		BrainModelsPath:     modelsPath,
		BrainDefaultModel:   "qwen9b",
		BrainBaseURL:        "http://127.0.0.1:9000",
		BrainIdleTTL:        time.Minute,
		BrainAutoWake:       true,
	}
	deps, cleanup, ok := buildBrainDeps(context.Background(), cfg,
		policy.NewEngine(policy.DefaultRuleset()), policy.NewApprovals(), s.DB)
	if !ok {
		t.Fatal("expected ok=true when enabled + configured")
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
