package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv   string
	HTTPPort string
	LogLevel string
	StateDir string
	DBPath   string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPPort: getEnv("HTTP_PORT", "8080"),
		LogLevel: getEnv("LOG_LEVEL", "debug"),
		StateDir: getEnv("STATE_DIR", "./state"),
		DBPath:   getEnv("DB_PATH", "./state/ptolemy.db"),
	}

	if cfg.HTTPPort == "" {
		return Config{}, fmt.Errorf("HTTP_PORT cannot be empty")
	}

	if err := ensureDir(cfg.StateDir); err != nil {
		return Config{}, fmt.Errorf("failed to ensure state dir: %w", err)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}