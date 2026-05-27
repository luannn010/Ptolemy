package memory

import (
	"testing"
)

func TestLoadConfig_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected DATABASE_URL to be required")
	}
}

func TestLoadConfig_ParsesAllFields(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/d?sslmode=disable")
	t.Setenv("EMBEDDING_BASE_URL", "http://embed:8000")
	t.Setenv("EMBEDDING_MODEL", "bge-large")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("EMBEDDING_API_KEY", "ek")
	t.Setenv("BRAIN_BASE_URL", "http://llm:8000")
	t.Setenv("BRAIN_MODEL", "qwen")
	t.Setenv("RAG_TOP_K", "8")
	t.Setenv("RAG_CHUNK_SIZE_TOKENS", "700")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "100")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://u:p@h:5432/d?sslmode=disable" {
		t.Fatalf("DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.EmbeddingDim != 1024 {
		t.Fatalf("EmbeddingDim: %d", cfg.EmbeddingDim)
	}
	if cfg.EmbeddingBaseURL != "http://embed:8000" || cfg.EmbeddingModel != "bge-large" || cfg.EmbeddingAPIKey != "ek" {
		t.Fatalf("embedding fields wrong: %+v", cfg)
	}
	if cfg.LLMBaseURL != "http://llm:8000" || cfg.LLMModel != "qwen" {
		t.Fatalf("llm fields wrong: %+v", cfg)
	}
	if cfg.TopK != 8 || cfg.ChunkSizeTokens != 700 || cfg.ChunkOverlapTokens != 100 {
		t.Fatalf("RAG knobs wrong: %+v", cfg)
	}
}

func TestLoadConfig_RejectsZeroEmbeddingDim(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/d")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "0")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected EMBEDDING_DIM=0 to be rejected")
	}
}

func TestLoadConfig_RequiresEmbeddingEndpoint(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error when EMBEDDING_BASE_URL is missing")
	}
}

func TestLoadConfig_RequiresBrainEndpoint(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error when BRAIN_BASE_URL is missing")
	}
}

func TestLoadConfig_RejectsNonIntegerEmbeddingDim(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "not-a-number")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error when EMBEDDING_DIM is non-integer")
	}
}

func TestIntEnv_FallsBackOnInvalidValue(t *testing.T) {
	t.Setenv("PTOLEMY_TEST_INT", "not-an-int")
	if got := intEnv("PTOLEMY_TEST_INT", 42); got != 42 {
		t.Fatalf("expected fallback 42 on non-integer, got %d", got)
	}
	t.Setenv("PTOLEMY_TEST_INT", "-5")
	if got := intEnv("PTOLEMY_TEST_INT", 42); got != 42 {
		t.Fatalf("expected fallback 42 on non-positive int, got %d", got)
	}
}

func TestLoadConfig_RAGKnobsDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_TOP_K", "")
	t.Setenv("RAG_CHUNK_SIZE_TOKENS", "")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TopK != 8 || cfg.ChunkSizeTokens != 700 || cfg.ChunkOverlapTokens != 100 {
		t.Fatalf("expected defaults 8/700/100, got %d/%d/%d", cfg.TopK, cfg.ChunkSizeTokens, cfg.ChunkOverlapTokens)
	}
}
