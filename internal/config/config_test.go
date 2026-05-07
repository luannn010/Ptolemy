package config

import (
	"os"
	"testing"
)

func TestLoadConfigWithEnv(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("DB_PATH", t.TempDir()+"/test.db")
	t.Setenv("WORKER_BASE_URL", "http://127.0.0.1:18080")
	t.Setenv("BRAIN_BASE_URL", "http://127.0.0.1:18088")
	t.Setenv("BRAIN_MODEL", "test-model")
	t.Setenv("PTOLEMY_AGENT_BIN", "/tmp/ptolemy-agent")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.AppEnv != "test" {
		t.Fatalf("expected APP_ENV test, got %s", cfg.AppEnv)
	}

	if cfg.HTTPPort != "9090" {
		t.Fatalf("expected HTTP_PORT 9090, got %s", cfg.HTTPPort)
	}

	if cfg.LogLevel != "info" {
		t.Fatalf("expected LOG_LEVEL info, got %s", cfg.LogLevel)
	}

	if _, err := os.Stat(cfg.StateDir); err != nil {
		t.Fatalf("expected state dir to exist: %v", err)
	}

	if cfg.WorkerBaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("expected WORKER_BASE_URL override, got %s", cfg.WorkerBaseURL)
	}

	if cfg.BrainBaseURL != "http://127.0.0.1:18088" {
		t.Fatalf("expected BRAIN_BASE_URL override, got %s", cfg.BrainBaseURL)
	}

	if cfg.BrainModel != "test-model" {
		t.Fatalf("expected BRAIN_MODEL override, got %s", cfg.BrainModel)
	}

	if cfg.AgentBinaryPath != "/tmp/ptolemy-agent" {
		t.Fatalf("expected PTOLEMY_AGENT_BIN override, got %s", cfg.AgentBinaryPath)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("STATE_DIR", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("WORKER_BASE_URL", "")
	t.Setenv("BRAIN_BASE_URL", "")
	t.Setenv("BRAIN_MODEL", "")
	t.Setenv("PTOLEMY_AGENT_BIN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Fatalf("expected default AppEnv development, got %s", cfg.AppEnv)
	}

	if cfg.HTTPPort != "8080" {
		t.Fatalf("expected default HTTPPort 8080, got %s", cfg.HTTPPort)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf("expected default LogLevel debug, got %s", cfg.LogLevel)
	}

	if cfg.WorkerBaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("expected default WorkerBaseURL http://127.0.0.1:8080, got %s", cfg.WorkerBaseURL)
	}

	if cfg.BrainBaseURL != "http://127.0.0.1:8088" {
		t.Fatalf("expected default BrainBaseURL http://127.0.0.1:8088, got %s", cfg.BrainBaseURL)
	}

	if cfg.BrainModel != "gemma-4-e2b" {
		t.Fatalf("expected default BrainModel gemma-4-e2b, got %s", cfg.BrainModel)
	}
}
