package memory

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// NewModule wires a production Orchestrator from MemoryConfig. It opens a pgx
// connection, applies migrations, and constructs every concrete implementation.
// Callers should hold on to the returned *pgx.Conn (so they can close it on
// shutdown) and the Orchestrator (the only thing they call into).
func NewModule(ctx context.Context, cfg MemoryConfig) (*Orchestrator, *pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := ApplyMigrations(ctx, conn, cfg.EmbeddingDim); err != nil {
		_ = conn.Close(ctx)
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	embedder := NewOpenAIEmbedder(cfg.EmbeddingBaseURL, cfg.EmbeddingModel, cfg.EmbeddingAPIKey)
	generator := NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, "")
	// Token→rune conversion: most English tokenizers land near 4 chars/token, and
	// we use rune-counts internally (see Chunker). The default 700-token /
	// 100-overlap config maps to ~2800/400 runes — slightly conservative, which
	// helps stay under embedding API per-request limits.
	const runesPerToken = 4
	return &Orchestrator{
		Chunker: FixedSizeChunker{
			MaxRunes: cfg.ChunkSizeTokens * runesPerToken,
			Overlap:  cfg.ChunkOverlapTokens * runesPerToken,
		},
		Embedder:       embedder,
		Store:          NewPgStore(conn),
		Retriever:      NewHybridRetriever(conn, embedder, cfg.RecencyWeight, cfg.RecencyHalfLife),
		Fusion:         PassthroughFusion{},
		ContextBuilder: MMRContextBuilder{Lambda: cfg.MMRLambda, K: cfg.TopK, MaxRunes: 6000},
		Generator:      generator,
		Depth:          20,
		FinalK:         cfg.TopK,
	}, conn, nil
}

// MaybeStartSweep loads MemoryConfig and, if GC_SWEEP_ENABLED is true AND a
// DATABASE_URL is configured, opens a pgx connection, applies migrations, and
// starts the sweep goroutine bound to ctx. Returns a cleanup func (closes the
// connection — call it on shutdown AFTER cancelling ctx) and an enabled flag.
//
// Never panics. A memory-side failure (bad config, unreachable DB) returns
// (nil, true, err) so the caller can log loudly and continue serving its core
// duties without the sweep.
func MaybeStartSweep(ctx context.Context) (cleanup func(), enabled bool, err error) {
	if !boolEnv("GC_SWEEP_ENABLED", false) {
		return nil, false, nil
	}
	// Sweep is explicitly enabled, so a missing DATABASE_URL is a
	// misconfiguration — surface it as an error (enabled=true) so the operator
	// sees "enabled but failed to start" rather than a silent "disabled".
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		return nil, true, fmt.Errorf("GC_SWEEP_ENABLED=true but DATABASE_URL is not set")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, true, fmt.Errorf("memory config: %w", err)
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, true, fmt.Errorf("connect postgres: %w", err)
	}
	if err := ApplyMigrations(ctx, conn, cfg.EmbeddingDim); err != nil {
		_ = conn.Close(ctx)
		return nil, true, fmt.Errorf("migrate: %w", err)
	}
	sw := NewSweeper(conn, cfg.GC)
	sweepCtx, cancelSweep := context.WithCancel(ctx)
	done := make(chan struct{})
	go sw.Run(sweepCtx, done)
	cleanup = func() {
		cancelSweep()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Error().Msg("memory sweep did not stop within 5s; closing connection anyway (may interrupt an in-flight write)")
		}
		_ = conn.Close(context.Background())
	}
	return cleanup, true, nil
}

// NewCaptureHookFromConfig builds (but does not start) the per-turn capture hook,
// constructing the BRAIN_* extractor from config. The caller starts it with
// hook.Start(ctx) and feeds it via hook.Enqueue. Wiring into the agent loop is
// out of 6a scope.
func NewCaptureHookFromConfig(cfg MemoryConfig, store Store, embedder Embedder) *PerTurnCaptureHook {
	chat := NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, "")
	return NewCaptureHook(NewExtractor(chat), embedder, store, cfg.CaptureBufferSize)
}
