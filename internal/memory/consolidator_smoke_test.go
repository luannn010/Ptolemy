//go:build smoke

package memory

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestConsolidatorSmoke runs the REAL BRAIN_* LLM through the consolidator's
// synthesize step against sample atoms. Build-tagged `smoke` so it never runs in
// the normal suite. Run via `make smoke-consolidate`.
func TestConsolidatorSmoke(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("BRAIN_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("BRAIN_MODEL"))
	if base == "" || model == "" {
		t.Skip("BRAIN_* not set")
	}
	c := NewConsolidator(nil, nil, NewOpenAIGenerator(base, model, ""), ConsolidateConfig{MinAtoms: 1})
	syn, err := c.synthesize(context.Background(), "",
		[]Chunk{{ID: "a1", Content: "the archive threshold is 0.1"}, {ID: "a2", Content: "the sweep runs hourly"}})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	t.Logf("summary=%q sources=%v", syn.Content, syn.SourceIDs)
	if strings.TrimSpace(syn.Content) == "" {
		t.Fatal("expected a non-empty summary from the real LLM")
	}
}
