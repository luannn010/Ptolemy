//go:build smoke

package memory

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestExtractorSmoke runs the REAL BRAIN_* extractor against a sample exchange.
// Build-tagged `smoke` so it never runs in the normal suite. Run via `make smoke-capture`.
func TestExtractorSmoke(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("BRAIN_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("BRAIN_MODEL"))
	if base == "" || model == "" {
		t.Skip("BRAIN_BASE_URL/BRAIN_MODEL not set")
	}
	e := NewExtractor(NewOpenAIGenerator(base, model, ""))
	ex := Exchange{
		UserText:      "we decided the GC archive threshold should be 0.1",
		AssistantText: "the sweep archives a project row when its decay score drops below 0.1",
		SubjectID:     "smoke", SessionID: "s", ProjectID: "ptolemy",
	}
	entries, err := e.Extract(context.Background(), ex)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	t.Logf("extractor returned %d entries", len(entries))
	for i, en := range entries {
		t.Logf("  [%d] perspective=%s fact=%s/%s content=%q", i, en.Perspective, en.FactSubject, en.FactPredicate, en.Content)
	}
	// Smoke = doesn't crash and returns valid (possibly empty) entries. No hard
	// count assertion (LLM output varies); a panic or parse error fails the test.
}
