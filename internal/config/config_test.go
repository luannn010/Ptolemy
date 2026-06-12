package config

import (
	"os"
	"testing"
	"time"
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
	t.Setenv("MCP_BASE_URL", "http://127.0.0.1:18081")
	t.Setenv("HEALTH_TIMEOUT_MS", "2500")

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
	if cfg.MCPBaseURL != "http://127.0.0.1:18081" {
		t.Fatalf("expected MCP_BASE_URL override, got %s", cfg.MCPBaseURL)
	}
	if cfg.HealthTimeoutMS != 2500 {
		t.Fatalf("expected HEALTH_TIMEOUT_MS override, got %d", cfg.HealthTimeoutMS)
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
	t.Setenv("MCP_BASE_URL", "")
	t.Setenv("HEALTH_TIMEOUT_MS", "")

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

	if cfg.WorkerBaseURL != DefaultWorkerBaseURL {
		t.Fatalf("expected default WorkerBaseURL %s, got %s", DefaultWorkerBaseURL, cfg.WorkerBaseURL)
	}

	if cfg.BrainBaseURL != DefaultBrainBaseURL {
		t.Fatalf("expected default BrainBaseURL %s, got %s", DefaultBrainBaseURL, cfg.BrainBaseURL)
	}

	if cfg.BrainModel != DefaultBrainModel {
		t.Fatalf("expected default BrainModel %s, got %s", DefaultBrainModel, cfg.BrainModel)
	}
	if cfg.MCPBaseURL != DefaultMCPBaseURL {
		t.Fatalf("expected default MCPBaseURL %q, got %q", DefaultMCPBaseURL, cfg.MCPBaseURL)
	}
	if cfg.HealthTimeoutMS <= 0 {
		t.Fatalf("expected positive HealthTimeoutMS, got %d", cfg.HealthTimeoutMS)
	}
}

func TestLoadConfigTrimsBlankEnvValues(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("WORKER_BASE_URL", "   ")
	t.Setenv("BRAIN_BASE_URL", "\t\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.WorkerBaseURL != DefaultWorkerBaseURL {
		t.Fatalf("expected default WorkerBaseURL for blank env, got %s", cfg.WorkerBaseURL)
	}

	if cfg.BrainBaseURL != DefaultBrainBaseURL {
		t.Fatalf("expected default BrainBaseURL for blank env, got %s", cfg.BrainBaseURL)
	}
}

func TestLoadConfigCarriesMemoryVars(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("EMBEDDING_API_KEY", "ek")
	t.Setenv("RAG_TOP_K", "12")
	t.Setenv("RAG_CHUNK_SIZE_TOKENS", "600")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "80")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://x" {
		t.Fatalf("DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.EmbeddingBaseURL != "http://e" || cfg.EmbeddingModel != "m" {
		t.Fatalf("embedding endpoint: %+v", cfg)
	}
	if cfg.EmbeddingDim != 768 || cfg.EmbeddingAPIKey != "ek" {
		t.Fatalf("embedding dim/key: %+v", cfg)
	}
	if cfg.RagTopK != 12 || cfg.RagChunkSize != 600 || cfg.RagChunkOverlap != 80 {
		t.Fatalf("RAG knobs: %+v", cfg)
	}
}

func TestLoadConfigBrainFields(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("BRAIN_CONTROL_ENABLED", "true")
	t.Setenv("BRAIN_AUTO_WAKE", "true")
	t.Setenv("BRAIN_IDLE_TTL", "300s")
	t.Setenv("BRAIN_CONTROL_PORT", "18089")
	t.Setenv("BRAIN_MODELS", "/tmp/brain-models.json")
	t.Setenv("BRAIN_DEFAULT_MODEL", "qwen9b")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.BrainControlEnabled || !cfg.BrainAutoWake {
		t.Fatalf("expected brain control+autowake enabled, got %+v", cfg)
	}
	if cfg.BrainIdleTTL != 300*time.Second {
		t.Fatalf("expected idle ttl 300s, got %v", cfg.BrainIdleTTL)
	}
	if cfg.BrainControlPort != "18089" || cfg.BrainModelsPath != "/tmp/brain-models.json" || cfg.BrainDefaultModel != "qwen9b" {
		t.Fatalf("brain string fields wrong: %+v", cfg)
	}
}

func TestLoadConfigBrainDefaults(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("BRAIN_CONTROL_ENABLED", "")
	t.Setenv("BRAIN_AUTO_WAKE", "")
	t.Setenv("BRAIN_IDLE_TTL", "")
	t.Setenv("BRAIN_CONTROL_PORT", "")
	t.Setenv("BRAIN_DEFAULT_MODEL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrainControlEnabled || cfg.BrainAutoWake {
		t.Fatalf("brain control must default off, got %+v", cfg)
	}
	if cfg.BrainIdleTTL != 5*time.Minute {
		t.Fatalf("expected default idle ttl 5m, got %v", cfg.BrainIdleTTL)
	}
	if cfg.BrainControlPort != "8089" {
		t.Fatalf("expected default control port 8089, got %q", cfg.BrainControlPort)
	}
}

func TestBoolAndDurationEnvFallBackOnGarbage(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("BRAIN_CONTROL_ENABLED", "not-a-bool")
	t.Setenv("BRAIN_IDLE_TTL", "not-a-duration")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrainControlEnabled {
		t.Fatal("garbage bool must fall back to false")
	}
	if cfg.BrainIdleTTL != 5*time.Minute {
		t.Fatalf("garbage duration must fall back to 5m, got %v", cfg.BrainIdleTTL)
	}
}

func TestLoadConfigRagPort(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("RAG_PORT", "18090")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RagPort != "18090" {
		t.Fatalf("expected RAG_PORT override 18090, got %q", cfg.RagPort)
	}

	t.Setenv("RAG_PORT", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RagPort != "8090" {
		t.Fatalf("expected default RagPort 8090, got %q", cfg.RagPort)
	}
}

func TestGetEnvIntFallsBackOnNonIntegerOrNegative(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("HEALTH_TIMEOUT_MS", "not-a-number")
	t.Setenv("RAG_TOP_K", "-7")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HealthTimeoutMS != 1500 {
		t.Fatalf("expected non-integer HEALTH_TIMEOUT_MS to fall back to 1500, got %d", cfg.HealthTimeoutMS)
	}
	if cfg.RagTopK != 8 {
		t.Fatalf("expected negative RAG_TOP_K to fall back to 8, got %d", cfg.RagTopK)
	}
}

func TestLoadConfigMemoryDefaults(t *testing.T) {
	t.Setenv("STATE_DIR", t.TempDir())
	t.Setenv("DATABASE_URL", "")
	t.Setenv("EMBEDDING_BASE_URL", "")
	t.Setenv("EMBEDDING_MODEL", "")
	t.Setenv("EMBEDDING_DIM", "")
	t.Setenv("RAG_TOP_K", "")
	t.Setenv("RAG_CHUNK_SIZE_TOKENS", "")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "" || cfg.EmbeddingBaseURL != "" {
		t.Fatalf("endpoint fields must default to empty (memory.LoadConfig owns the strict validation)")
	}
	if cfg.RagTopK != 8 || cfg.RagChunkSize != 700 || cfg.RagChunkOverlap != 100 {
		t.Fatalf("RAG defaults wrong: %d/%d/%d", cfg.RagTopK, cfg.RagChunkSize, cfg.RagChunkOverlap)
	}
}
