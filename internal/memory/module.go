package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
		Retriever:      NewHybridRetriever(conn, embedder, 0.1, 30*24*time.Hour),
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Generator:      generator,
		Depth:          20,
		FinalK:         cfg.TopK,
	}, conn, nil
}
