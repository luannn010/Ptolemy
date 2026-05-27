package memory

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MemoryConfig is loaded entirely from environment variables, reusing the
// names already defined in .env.example. There are no defaults for service
// endpoints (DATABASE_URL, EMBEDDING_*, BRAIN_*): a missing endpoint is a
// misconfiguration that should fail fast at startup, not silently fall back.
// The RAG knobs (TopK, chunk sizes) have sensible defaults matching .env.example.
type MemoryConfig struct {
	DatabaseURL string

	EmbeddingBaseURL string
	EmbeddingModel   string
	EmbeddingDim     int
	EmbeddingAPIKey  string

	// LLM endpoint reuses the existing BRAIN_BASE_URL / BRAIN_MODEL env vars.
	LLMBaseURL string
	LLMModel   string

	TopK               int
	ChunkSizeTokens    int
	ChunkOverlapTokens int
}

func LoadConfig() (MemoryConfig, error) {
	cfg := MemoryConfig{
		DatabaseURL:      strings.TrimSpace(os.Getenv("DATABASE_URL")),
		EmbeddingBaseURL: strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")),
		EmbeddingModel:   strings.TrimSpace(os.Getenv("EMBEDDING_MODEL")),
		EmbeddingAPIKey:  strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")),
		LLMBaseURL:       strings.TrimSpace(os.Getenv("BRAIN_BASE_URL")),
		LLMModel:         strings.TrimSpace(os.Getenv("BRAIN_MODEL")),
	}

	if cfg.DatabaseURL == "" {
		return MemoryConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.EmbeddingBaseURL == "" || cfg.EmbeddingModel == "" {
		return MemoryConfig{}, fmt.Errorf("EMBEDDING_BASE_URL and EMBEDDING_MODEL are required")
	}
	if cfg.LLMBaseURL == "" || cfg.LLMModel == "" {
		return MemoryConfig{}, fmt.Errorf("BRAIN_BASE_URL and BRAIN_MODEL are required")
	}

	dimStr := strings.TrimSpace(os.Getenv("EMBEDDING_DIM"))
	dim, err := strconv.Atoi(dimStr)
	if err != nil || dim <= 0 {
		return MemoryConfig{}, fmt.Errorf("EMBEDDING_DIM must be a positive integer, got %q", dimStr)
	}
	cfg.EmbeddingDim = dim

	cfg.TopK = intEnv("RAG_TOP_K", 8)
	cfg.ChunkSizeTokens = intEnv("RAG_CHUNK_SIZE_TOKENS", 700)
	cfg.ChunkOverlapTokens = intEnv("RAG_CHUNK_OVERLAP_TOKENS", 100)

	return cfg, nil
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
