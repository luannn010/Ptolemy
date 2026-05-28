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

	// GC knobs (Phase 4). Defaults are placeholders to be tuned against real
	// chat volume once Phase 6 lands.
	GC GCConfig

	// Phase 6a capture/recall knobs.
	CaptureBufferSize int     // CAPTURE_BUFFER_SIZE (default 256; 0/garbage → default)
	MMRLambda         float64 // RAG_MMR_LAMBDA (default 0.7; must be in [0,1])
}

// GCConfig holds the garbage-collection knobs loaded from env vars.
type GCConfig struct {
	SweepEnabled     bool          // GC_SWEEP_ENABLED (default false)
	SweepInterval    time.Duration // GC_SWEEP_INTERVAL (default 1h)
	DecayLambda      float64       // GC_DECAY_LAMBDA (default 0.05)
	ArchiveThreshold float64       // GC_ARCHIVE_THRESHOLD (default 0.1)
	PurgeEnabled     bool          // GC_PURGE_ENABLED (default false)
	PurgeGraceDays   float64       // GC_PURGE_GRACE_DAYS (default 30)

	// Phase 5 dedup knobs (placeholders — tune against real volume).
	DedupEnabled   bool          // GC_DEDUP_ENABLED (default false)
	DedupThreshold float64       // GC_DEDUP_THRESHOLD trigram candidate prefilter (default 0.7)
	DedupLookback  time.Duration // GC_DEDUP_LOOKBACK recent-row window (default 24h)
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

	cfg.GC = GCConfig{
		SweepEnabled:     boolEnv("GC_SWEEP_ENABLED", false),
		SweepInterval:    durationEnv("GC_SWEEP_INTERVAL", time.Hour),
		DecayLambda:      floatEnv("GC_DECAY_LAMBDA", 0.05),
		ArchiveThreshold: floatEnv("GC_ARCHIVE_THRESHOLD", 0.1),
		PurgeEnabled:     boolEnv("GC_PURGE_ENABLED", false),
		PurgeGraceDays:   floatEnv("GC_PURGE_GRACE_DAYS", 30),
		DedupEnabled:     boolEnv("GC_DEDUP_ENABLED", false),
		DedupThreshold:   floatEnv("GC_DEDUP_THRESHOLD", 0.7),
		DedupLookback:    durationEnv("GC_DEDUP_LOOKBACK", 24*time.Hour),
	}
	if cfg.GC.DecayLambda < 0 {
		return MemoryConfig{}, fmt.Errorf("GC_DECAY_LAMBDA must be >= 0, got %v", cfg.GC.DecayLambda)
	}
	if cfg.GC.ArchiveThreshold <= 0 || cfg.GC.ArchiveThreshold > 1 {
		return MemoryConfig{}, fmt.Errorf("GC_ARCHIVE_THRESHOLD must be in (0,1], got %v", cfg.GC.ArchiveThreshold)
	}
	if cfg.GC.SweepInterval < time.Second {
		return MemoryConfig{}, fmt.Errorf("GC_SWEEP_INTERVAL must be >= 1s, got %v", cfg.GC.SweepInterval)
	}
	if cfg.GC.PurgeGraceDays < 1 {
		return MemoryConfig{}, fmt.Errorf("GC_PURGE_GRACE_DAYS must be >= 1, got %v", cfg.GC.PurgeGraceDays)
	}
	if cfg.GC.DedupThreshold <= 0 || cfg.GC.DedupThreshold > 1 {
		return MemoryConfig{}, fmt.Errorf("GC_DEDUP_THRESHOLD must be in (0,1], got %v", cfg.GC.DedupThreshold)
	}
	if cfg.GC.DedupLookback < time.Minute {
		return MemoryConfig{}, fmt.Errorf("GC_DEDUP_LOOKBACK must be >= 1m, got %v", cfg.GC.DedupLookback)
	}

	cfg.CaptureBufferSize = intEnv("CAPTURE_BUFFER_SIZE", 256)
	cfg.MMRLambda = floatEnv("RAG_MMR_LAMBDA", 0.7)
	if cfg.MMRLambda < 0 || cfg.MMRLambda > 1 {
		return MemoryConfig{}, fmt.Errorf("RAG_MMR_LAMBDA must be in [0,1], got %v", cfg.MMRLambda)
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

func boolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}
