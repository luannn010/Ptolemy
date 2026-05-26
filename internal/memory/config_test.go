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
