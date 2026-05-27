package memory

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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

	// Recency tuning knobs (Phase 3). Spec defaults are 0.1 and 30 days;
	// production behavior is preserved when RAG_RECENCY_* are unset.
	RecencyWeight   float64       // env: RAG_RECENCY_WEIGHT
	RecencyHalfLife time.Duration // env: RAG_RECENCY_HALFLIFE_DAYS (parsed as float days)
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

	cfg.RecencyWeight = floatEnv("RAG_RECENCY_WEIGHT", 0.1)
	if cfg.RecencyWeight < 0 {
		return MemoryConfig{}, fmt.Errorf("RAG_RECENCY_WEIGHT must be >= 0, got %v", cfg.RecencyWeight)
	}
	halflifeDays := floatEnv("RAG_RECENCY_HALFLIFE_DAYS", 30)
	cfg.RecencyHalfLife = time.Duration(halflifeDays * float64(24*time.Hour))
	if cfg.RecencyHalfLife < time.Hour {
		return MemoryConfig{}, fmt.Errorf("RAG_RECENCY_HALFLIFE_DAYS resolves to %v, must be >= 1h", cfg.RecencyHalfLife)
	}

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

func floatEnv(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return v
}
