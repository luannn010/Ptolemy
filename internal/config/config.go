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
	MCPBaseURL      string
	HealthTimeoutMS int
}

const (
	DefaultWorkerBaseURL = "http://127.0.0.1:8080"
	DefaultBrainBaseURL  = "http://127.0.0.1:8088"
	DefaultBrainModel    = "gemma-4-e2b"
	DefaultMCPBaseURL    = ""
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
		MCPBaseURL:      getEnv("MCP_BASE_URL", DefaultMCPBaseURL),
		HealthTimeoutMS: getEnvInt("HEALTH_TIMEOUT_MS", 1500),
	}

	if cfg.HTTPPort == "" {
		return Config{}, fmt.Errorf("HTTP_PORT cannot be empty")
	}

	if err := ensureDir(cfg.StateDir); err != nil {
		return Config{}, fmt.Errorf("failed to ensure state dir: %w", err)
	}

	return cfg, nil
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var parsed int
	_, err := fmt.Sscanf(value, "%d", &parsed)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
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
