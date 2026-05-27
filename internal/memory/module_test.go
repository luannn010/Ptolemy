package memory

import (
	"context"
	"strings"
	"testing"
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
