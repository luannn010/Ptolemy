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
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("STATE_DIR", "")
	t.Setenv("DB_PATH", "")

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
}
