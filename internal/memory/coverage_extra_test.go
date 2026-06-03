package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMaybeStartConsolidator_DisabledWhenUnset(t *testing.T) {
	t.Setenv("CONSOLIDATE_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")

	cleanup, enabled, err := MaybeStartConsolidator(context.Background())
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

func TestMaybeStartConsolidator_EnabledWithoutDatabaseURLErrors(t *testing.T) {
	t.Setenv("CONSOLIDATE_ENABLED", "true")
	t.Setenv("DATABASE_URL", "")

	cleanup, enabled, err := MaybeStartConsolidator(context.Background())
	if err == nil {
		t.Fatal("expected an error when enabled without DATABASE_URL")
	}
	if !enabled {
		t.Fatal("expected enabled=true (operator opted in)")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup on error path")
	}
}

func TestMaybeStartConsolidator_InvalidMemoryConfigErrorsBeforeConnect(t *testing.T) {
	t.Setenv("CONSOLIDATE_ENABLED", "true")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("EMBEDDING_BASE_URL", "")
	t.Setenv("EMBEDDING_MODEL", "")

	_, enabled, err := MaybeStartConsolidator(context.Background())
	if err == nil {
		t.Fatal("expected memory config error")
	}
	if !enabled {
		t.Fatal("expected enabled=true when CONSOLIDATE_ENABLED=true")
	}
	if !strings.Contains(err.Error(), "memory config") {
		t.Fatalf("expected wrapped memory config error, got %v", err)
	}
}

func TestMaybeStartSweep_InvalidMemoryConfigErrorsBeforeConnect(t *testing.T) {
	t.Setenv("GC_SWEEP_ENABLED", "true")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("EMBEDDING_BASE_URL", "")
	t.Setenv("EMBEDDING_MODEL", "")

	_, enabled, err := MaybeStartSweep(context.Background())
	if err == nil {
		t.Fatal("expected memory config error")
	}
	if !enabled {
		t.Fatal("expected enabled=true when GC_SWEEP_ENABLED=true")
	}
	if !strings.Contains(err.Error(), "memory config") {
		t.Fatalf("expected wrapped memory config error, got %v", err)
	}
}

func TestNewCaptureHookFromConfig_UsesConfiguredBuffer(t *testing.T) {
	cfg := MemoryConfig{
		LLMBaseURL:         "http://llm",
		LLMModel:           "test-model",
		CaptureBufferSize:  17,
		EmbeddingBaseURL:   "http://embed",
		EmbeddingModel:     "embed-model",
		ChunkSizeTokens:    10,
		ChunkOverlapTokens: 1,
	}
	hook := NewCaptureHookFromConfig(cfg, &fakeStore{}, fakeEmbedder{vecs: [][]float32{{1}}})
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}
	if cap(hook.ch) != 17 {
		t.Fatalf("expected capture channel capacity 17, got %d", cap(hook.ch))
	}
	if hook.extractor == nil || hook.chain == nil || hook.metrics == nil {
		t.Fatal("expected hook internals to be initialized")
	}
}

func TestPerTurnCaptureHook_CaptureDelegatesToProcessTurn(t *testing.T) {
	ex := NewExtractor(fakeChat{resp: `{"atoms":[{"content":"the sweep archives stale rows","perspective":"factual"}]}`})
	store := &fakeStore{}
	hook := NewCaptureHook(ex, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, store, 1)

	err := hook.Capture(context.Background(), Exchange{
		UserText:      "what does the sweep do?",
		AssistantText: "the sweep archives stale rows",
		SessionID:     "s1",
		SubjectID:     "u1",
		ProjectID:     "p1",
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if hook.Metrics().Captured() != 1 {
		t.Fatalf("expected captured metric 1, got %d", hook.Metrics().Captured())
	}
}

func TestPerTurnCaptureHook_StartProcessesEnqueuedExchange(t *testing.T) {
	ex := NewExtractor(fakeChat{resp: `{"atoms":[{"content":"sweep archives stale rows","perspective":"factual"}]}`})
	store := &fakeStore{}
	hook := NewCaptureHook(ex, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, store, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hook.Start(ctx)

	hook.Enqueue(Exchange{
		UserText:      "what does sweep do",
		AssistantText: "sweep archives stale rows",
		SessionID:     "s1",
		SubjectID:     "u1",
		ProjectID:     "p1",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hook.Metrics().Captured() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected Start worker to process enqueued exchange")
}

func TestConsolidator_WithEmbedder_SetsAndReturnsSelf(t *testing.T) {
	c := NewConsolidator(nil, &fakeStore{}, nil, ConsolidateConfig{})
	emb := fakeEmbedder{vecs: [][]float32{{1, 2}}}
	got := c.WithEmbedder(emb)
	if got != c {
		t.Fatal("WithEmbedder should return same consolidator pointer for chaining")
	}
	if c.embedder == nil {
		t.Fatal("embedder should be set")
	}
}

func TestSweeper_NoConnShortCircuitsWhenPassesDisabled(t *testing.T) {
	s := NewSweeper(nil, GCConfig{
		DedupEnabled: false,
		PurgeEnabled: false,
	})
	if err := s.dedupRecent(context.Background()); err != nil {
		t.Fatalf("dedup disabled should short-circuit without DB, got %v", err)
	}
	if err := s.purgeDead(context.Background()); err != nil {
		t.Fatalf("purge disabled should short-circuit without DB, got %v", err)
	}
}

func TestMeasureDedupCollapses_NormalizedDuplicatesOnly(t *testing.T) {
	docs := []RawDocument{
		{ID: "1", Text: "User prefers tabs"},
		{ID: "2", Text: "User   prefers   tabs"},
		{ID: "3", Text: "Different preference"},
		{ID: "4", Text: "Different   preference"},
	}
	got := MeasureDedupCollapses(docs)
	if got != 2 {
		t.Fatalf("expected 2 collapses, got %d", got)
	}
}

func TestValidatorNames_AreStable(t *testing.T) {
	if got := (NonEmptyValidator{}).Name(); got != "non_empty" {
		t.Fatalf("unexpected non-empty validator name: %q", got)
	}
	if got := (LengthBoundsValidator{}).Name(); got != "length_bounds" {
		t.Fatalf("unexpected length-bounds validator name: %q", got)
	}
	if got := (PredicateTaxonomyValidator{}).Name(); got != "predicate_taxonomy" {
		t.Fatalf("unexpected predicate validator name: %q", got)
	}
	if got := (EvidenceInSourceValidator{}).Name(); got != "evidence_in_source" {
		t.Fatalf("unexpected evidence validator name: %q", got)
	}
}

func TestCaptureMetrics_DefaultsAndAccessors(t *testing.T) {
	var m CaptureMetrics
	if m.Dropped() != 0 || m.ExtractErr() != 0 || m.EmbedErr() != 0 || m.Captured() != 0 {
		t.Fatalf("zero-value metrics should all be zero")
	}
}

func TestDurationEnv_ParsesAndFallsBack(t *testing.T) {
	t.Setenv("PTOLEMY_TEST_DUR", "2s")
	if got := durationEnv("PTOLEMY_TEST_DUR", time.Minute); got != 2*time.Second {
		t.Fatalf("expected parsed duration 2s, got %v", got)
	}
	t.Setenv("PTOLEMY_TEST_DUR", "invalid")
	if got := durationEnv("PTOLEMY_TEST_DUR", time.Minute); got != time.Minute {
		t.Fatalf("expected fallback duration 1m, got %v", got)
	}
}

func TestHybridRetriever_FluentSettersAndPreview(t *testing.T) {
	r := NewHybridRetriever(nil, nil, 0.1, 24*time.Hour)
	if got := r.WithDecayLambda(0.25); got != r {
		t.Fatal("WithDecayLambda should return same receiver")
	}
	if r.decayLambda != 0.25 {
		t.Fatalf("expected decay lambda override to stick, got %v", r.decayLambda)
	}
	r.WithAliasExpansion(true)
	if !r.aliasExpansion {
		t.Fatal("expected alias expansion flag to be true")
	}

	short := preview("line1\nline2", 20)
	if strings.Contains(short, "\n") || strings.Contains(short, "\r") {
		t.Fatalf("preview should collapse newlines, got %q", short)
	}
	long := preview("abcdefghijklmnopqrstuvwxyz", 5)
	if !strings.HasPrefix(long, "abcde") {
		t.Fatalf("preview prefix mismatch: %q", long)
	}
}

func TestStoreHelperDefaults(t *testing.T) {
	if got := scopeOrDefault(""); got != "global" {
		t.Fatalf("nil scope should default to global, got %q", got)
	}
	if got := scopeOrDefault("project"); got != "project" {
		t.Fatalf("explicit scope should be preserved, got %q", got)
	}

	if got := confidenceOrDefault(""); got != "normal" {
		t.Fatalf("empty confidence should default to normal, got %q", got)
	}
	if got := confidenceOrDefault("0.6"); got != "0.6" {
		t.Fatalf("explicit confidence should be preserved, got %q", got)
	}

	if got := importanceOrDefault(0); got != 1.0 {
		t.Fatalf("nil importance should default to 1.0, got %v", got)
	}
	if got := importanceOrDefault(0.7); got != 0.7 {
		t.Fatalf("explicit importance should be preserved, got %v", got)
	}
}
