package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv          string
	HTTPPort        string
	LogLevel        string
	StateDir        string
	DBPath          string
	WorkerBaseURL   string
	BrainBaseURL    string
	BrainModel      string
	AgentBinaryPath string
}

const (
	DefaultWorkerBaseURL = "http://127.0.0.1:8080"
	DefaultBrainBaseURL  = "http://127.0.0.1:8088"
	DefaultBrainModel    = "gemma-4-e2b"
)

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:          getEnv("APP_ENV", "development"),
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "debug"),
		StateDir:        getEnv("STATE_DIR", "./state"),
		DBPath:          getEnv("DB_PATH", "./state/ptolemy.db"),
		WorkerBaseURL:   getEnv("WORKER_BASE_URL", DefaultWorkerBaseURL),
		BrainBaseURL:    getEnv("BRAIN_BASE_URL", DefaultBrainBaseURL),
		BrainModel:      getEnv("BRAIN_MODEL", DefaultBrainModel),
		AgentBinaryPath: getEnv("PTOLEMY_AGENT_BIN", ""),
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
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
