package memory

import (
	"testing"
	"time"
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

func TestLoadConfig_RecencyDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_WEIGHT", "")
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RecencyWeight != 0.1 {
		t.Fatalf("default weight: got %v want 0.1", cfg.RecencyWeight)
	}
	wantHL := 30 * 24 * time.Hour
	if cfg.RecencyHalfLife != wantHL {
		t.Fatalf("default halflife: got %v want %v", cfg.RecencyHalfLife, wantHL)
	}
}

func TestLoadConfig_RecencyEnvParsed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_WEIGHT", "0.2")
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "7")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RecencyWeight != 0.2 {
		t.Fatalf("weight: got %v want 0.2", cfg.RecencyWeight)
	}
	if cfg.RecencyHalfLife != 7*24*time.Hour {
		t.Fatalf("halflife: got %v want %v", cfg.RecencyHalfLife, 7*24*time.Hour)
	}
}

func TestLoadConfig_RejectsNegativeRecencyWeight(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_WEIGHT", "-0.1")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected negative weight to be rejected")
	}
}

func TestLoadConfig_RejectsHalfLifeBelow1Hour(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	// 0.01 days = 14.4 minutes
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "0.01")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected sub-1h halflife to be rejected")
	}
}

func TestLoadConfig_RejectsZeroHalfLife(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "0")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected zero halflife to be rejected")
	}
}

func TestLoadConfig_GCDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	for _, k := range []string{"GC_SWEEP_ENABLED", "GC_SWEEP_INTERVAL", "GC_DECAY_LAMBDA",
		"GC_ARCHIVE_THRESHOLD", "GC_PURGE_ENABLED", "GC_PURGE_GRACE_DAYS"} {
		t.Setenv(k, "")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GC.SweepEnabled || cfg.GC.PurgeEnabled {
		t.Fatalf("enables should default false: %+v", cfg.GC)
	}
	if cfg.GC.SweepInterval != time.Hour {
		t.Fatalf("interval default 1h, got %v", cfg.GC.SweepInterval)
	}
	if cfg.GC.DecayLambda != 0.05 || cfg.GC.ArchiveThreshold != 0.1 || cfg.GC.PurgeGraceDays != 30 {
		t.Fatalf("GC numeric defaults wrong: %+v", cfg.GC)
	}
}

func TestLoadConfig_GCEnvParsed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("GC_SWEEP_ENABLED", "true")
	t.Setenv("GC_SWEEP_INTERVAL", "5s")
	t.Setenv("GC_DECAY_LAMBDA", "0.1")
	t.Setenv("GC_ARCHIVE_THRESHOLD", "0.2")
	t.Setenv("GC_PURGE_ENABLED", "true")
	t.Setenv("GC_PURGE_GRACE_DAYS", "7")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GC.SweepEnabled || !cfg.GC.PurgeEnabled {
		t.Fatalf("enables should be true: %+v", cfg.GC)
	}
	if cfg.GC.SweepInterval != 5*time.Second {
		t.Fatalf("interval: got %v", cfg.GC.SweepInterval)
	}
	if cfg.GC.DecayLambda != 0.1 || cfg.GC.ArchiveThreshold != 0.2 || cfg.GC.PurgeGraceDays != 7 {
		t.Fatalf("GC numerics: %+v", cfg.GC)
	}
}

func TestGCConfig_DedupDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://x")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("BRAIN_BASE_URL", "http://x")
	t.Setenv("BRAIN_MODEL", "m")
	// Clear the dedup vars so an ambient shell value can't mask the defaults.
	for _, k := range []string{"GC_DEDUP_ENABLED", "GC_DEDUP_THRESHOLD", "GC_DEDUP_LOOKBACK"} {
		t.Setenv(k, "")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.GC.DedupEnabled {
		t.Errorf("DedupEnabled default = true, want false")
	}
	if cfg.GC.DedupThreshold != 0.7 {
		t.Errorf("DedupThreshold default = %v, want 0.7", cfg.GC.DedupThreshold)
	}
	if cfg.GC.DedupLookback != 24*time.Hour {
		t.Errorf("DedupLookback default = %v, want 24h", cfg.GC.DedupLookback)
	}
}

func TestGCConfig_RejectsBadDedup(t *testing.T) {
	base := func() {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("EMBEDDING_BASE_URL", "http://x")
		t.Setenv("EMBEDDING_MODEL", "m")
		t.Setenv("EMBEDDING_DIM", "768")
		t.Setenv("BRAIN_BASE_URL", "http://x")
		t.Setenv("BRAIN_MODEL", "m")
	}
	base()
	t.Setenv("GC_DEDUP_THRESHOLD", "1.5")
	if _, err := LoadConfig(); err == nil {
		t.Error("expected error for GC_DEDUP_THRESHOLD > 1")
	}
	base()
	t.Setenv("GC_DEDUP_THRESHOLD", "0")
	if _, err := LoadConfig(); err == nil {
		t.Error("expected error for GC_DEDUP_THRESHOLD = 0")
	}
	base()
	t.Setenv("GC_DEDUP_THRESHOLD", "0.7")
	t.Setenv("GC_DEDUP_LOOKBACK", "30s")
	if _, err := LoadConfig(); err == nil {
		t.Error("expected error for GC_DEDUP_LOOKBACK < 1m")
	}
}

func TestLoadConfig_CaptureAndMMRDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("CAPTURE_BUFFER_SIZE", "")
	t.Setenv("RAG_MMR_LAMBDA", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureBufferSize != 256 {
		t.Errorf("CaptureBufferSize default = %d, want 256", cfg.CaptureBufferSize)
	}
	if cfg.MMRLambda != 0.7 {
		t.Errorf("MMRLambda default = %v, want 0.7", cfg.MMRLambda)
	}
}

func TestLoadConfig_GCRejectsBadValues(t *testing.T) {
	base := func() {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("EMBEDDING_BASE_URL", "http://e")
		t.Setenv("EMBEDDING_MODEL", "m")
		t.Setenv("EMBEDDING_DIM", "1024")
		t.Setenv("BRAIN_BASE_URL", "http://l")
		t.Setenv("BRAIN_MODEL", "lm")
	}
	cases := map[string]map[string]string{
		"negative lambda":    {"GC_DECAY_LAMBDA": "-0.1"},
		"threshold over 1":   {"GC_ARCHIVE_THRESHOLD": "1.5"},
		"threshold zero":     {"GC_ARCHIVE_THRESHOLD": "0"},
		"interval too small": {"GC_SWEEP_INTERVAL": "500ms"}, // < 1s floor
		"grace under 1 day":  {"GC_PURGE_GRACE_DAYS": "0"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			base()
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := LoadConfig(); err == nil {
				t.Fatalf("%s: expected error", name)
			}
		})
	}
}

func TestLoadConfig_MMRLambdaRange(t *testing.T) {
	base := func() {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("EMBEDDING_BASE_URL", "http://e")
		t.Setenv("EMBEDDING_MODEL", "m")
		t.Setenv("EMBEDDING_DIM", "1024")
		t.Setenv("BRAIN_BASE_URL", "http://l")
		t.Setenv("BRAIN_MODEL", "lm")
	}
	reject := []string{"-0.1", "1.5"}
	for _, v := range reject {
		t.Run("reject "+v, func(t *testing.T) {
			base()
			t.Setenv("RAG_MMR_LAMBDA", v)
			if _, err := LoadConfig(); err == nil {
				t.Fatalf("RAG_MMR_LAMBDA=%s: expected error", v)
			}
		})
	}
	accept := []string{"0", "1"}
	for _, v := range accept {
		t.Run("accept "+v, func(t *testing.T) {
			base()
			t.Setenv("RAG_MMR_LAMBDA", v)
			if _, err := LoadConfig(); err != nil {
				t.Fatalf("RAG_MMR_LAMBDA=%s: unexpected error %v", v, err)
			}
		})
	}
}
