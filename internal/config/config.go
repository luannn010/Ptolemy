package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv          string
	HTTPPort        string
	ApprovePort     string
	RagPort         string // RAG HTTP listener (POST /chat); binds all interfaces
	LogLevel        string
	StateDir        string
	DBPath          string
	PolicyPath      string
	WorkerBaseURL   string
	BrainBaseURL    string
	BrainModel      string
	AgentBinaryPath string
	MCPBaseURL      string
	HealthTimeoutMS int
	DatabaseURL     string
	EmbeddingBaseURL string
	EmbeddingModel  string
	EmbeddingDim    int
	EmbeddingAPIKey string
	RagTopK         int
	RagChunkSize    int
	RagChunkOverlap int
	// Brain lifecycle skill (workerd-managed llama.cpp). Off by default.
	BrainControlEnabled bool
	BrainAutoWake       bool
	BrainIdleTTL        time.Duration
	BrainControlPort    string
	BrainModelsPath     string
	BrainDefaultModel   string
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
		ApprovePort:     getEnv("APPROVE_PORT", "8081"),
		RagPort:         getEnv("RAG_PORT", "8090"),
		LogLevel:        getEnv("LOG_LEVEL", "debug"),
		StateDir:        getEnv("STATE_DIR", "./state"),
		DBPath:          getEnv("DB_PATH", "./state/ptolemy.db"),
		PolicyPath:      getEnv("POLICY_PATH", "./.ptolemy/policy.json"),
		WorkerBaseURL:   getEnv("WORKER_BASE_URL", DefaultWorkerBaseURL),
		BrainBaseURL:    getEnv("BRAIN_BASE_URL", DefaultBrainBaseURL),
		BrainModel:      getEnv("BRAIN_MODEL", DefaultBrainModel),
		AgentBinaryPath: getEnv("PTOLEMY_AGENT_BIN", ""),
		MCPBaseURL:      getEnv("MCP_BASE_URL", DefaultMCPBaseURL),
		HealthTimeoutMS: getEnvInt("HEALTH_TIMEOUT_MS", 1500),
	}

	cfg.DatabaseURL = getEnv("DATABASE_URL", "")
	cfg.EmbeddingBaseURL = getEnv("EMBEDDING_BASE_URL", "")
	cfg.EmbeddingModel = getEnv("EMBEDDING_MODEL", "")
	cfg.EmbeddingDim = getEnvInt("EMBEDDING_DIM", 0)
	cfg.EmbeddingAPIKey = getEnv("EMBEDDING_API_KEY", "")
	cfg.RagTopK = getEnvInt("RAG_TOP_K", 8)
	cfg.RagChunkSize = getEnvInt("RAG_CHUNK_SIZE_TOKENS", 700)
	cfg.RagChunkOverlap = getEnvInt("RAG_CHUNK_OVERLAP_TOKENS", 100)

	cfg.BrainControlEnabled = getEnvBool("BRAIN_CONTROL_ENABLED", false)
	cfg.BrainAutoWake = getEnvBool("BRAIN_AUTO_WAKE", false)
	cfg.BrainIdleTTL = getEnvDuration("BRAIN_IDLE_TTL", 5*time.Minute)
	cfg.BrainControlPort = getEnv("BRAIN_CONTROL_PORT", "8089")
	cfg.BrainModelsPath = getEnv("BRAIN_MODELS", "")
	cfg.BrainDefaultModel = getEnv("BRAIN_DEFAULT_MODEL", "")

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

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	v, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return v
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	v, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return v
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
