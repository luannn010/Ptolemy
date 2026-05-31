package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewModule_FailsOnUnreachableDatabaseURL(t *testing.T) {
	// pgx.Connect against a port that is guaranteed closed must surface as a
	// wrapped "connect postgres" error rather than panicking or hanging.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := MemoryConfig{
		DatabaseURL:      "postgres://ptolemy:ptolemy@127.0.0.1:1/ptolemy?sslmode=disable&connect_timeout=1",
		EmbeddingBaseURL: "http://embed",
		EmbeddingModel:   "m",
		EmbeddingDim:     4,
		LLMBaseURL:       "http://llm",
		LLMModel:         "lm",
	}
	_, _, err := NewModule(ctx, cfg)
	if err == nil {
		t.Fatalf("expected NewModule to fail against unreachable DB")
	}
	if !strings.Contains(err.Error(), "connect postgres") {
		t.Fatalf("expected wrapped 'connect postgres' error, got %v", err)
	}
}

func TestNewModule_DefaultRetrieverIsHybrid(t *testing.T) {
	url := requirePG(t)
	cfg := MemoryConfig{
		DatabaseURL:        url,
		EmbeddingBaseURL:   "http://example.invalid",
		EmbeddingModel:     "fake",
		EmbeddingDim:       4,
		LLMBaseURL:         "http://example.invalid",
		LLMModel:           "fake",
		TopK:               5,
		ChunkSizeTokens:    50,
		ChunkOverlapTokens: 10,
		RecencyWeight:      0.1,
		RecencyHalfLife:    30 * 24 * time.Hour,
	}
	orch, conn, err := NewModule(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	defer conn.Close(context.Background())

	if _, ok := orch.Retriever.(*HybridRetriever); !ok {
		t.Fatalf("expected default Retriever to be *HybridRetriever, got %T", orch.Retriever)
	}
}

func TestMaybeStartSweep_DisabledWhenUnset(t *testing.T) {
	// No GC_SWEEP_ENABLED → disabled, regardless of DATABASE_URL.
	t.Setenv("GC_SWEEP_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	cleanup, enabled, err := MaybeStartSweep(context.Background())
	if err != nil {
		t.Fatalf("disabled path should not error, got %v", err)
	}
	if enabled {
		t.Fatal("expected enabled=false")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup when disabled")
	}
}

func TestMaybeStartSweep_EnabledWithoutDatabaseURLErrors(t *testing.T) {
	// Explicitly enabled but no DATABASE_URL → enabled=true + error, so the
	// operator sees a failure rather than a silent disable.
	t.Setenv("GC_SWEEP_ENABLED", "true")
	t.Setenv("DATABASE_URL", "")
	cleanup, enabled, err := MaybeStartSweep(context.Background())
	if err == nil {
		t.Fatal("expected an error when enabled without DATABASE_URL")
	}
	if !enabled {
		t.Fatal("expected enabled=true (operator opted in)")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup on the error path")
	}
}

func TestSweeper_Run_SignalsDoneOnCancel(t *testing.T) {
	sw := NewSweeper(nil, GCConfig{SweepInterval: time.Hour}) // ticker never fires within the test
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go sw.Run(ctx, done)
	cancel()
	select {
	case <-done:
		// ok: Run returned and closed done
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not signal done within 2s of cancel")
	}
}
