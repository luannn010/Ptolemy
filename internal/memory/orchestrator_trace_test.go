package memory

import (
	"context"
	"testing"
)

func TestOrchestratorAnswer_LegacyTrace(t *testing.T) {
	gen := &stubGenerator{text: "answer [source:a#0]", cites: []string{"a#0"}}
	o := &Orchestrator{
		Retriever:      stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", "grounded fact", 0.9)}},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Generator:      gen,
		// AgentLoop nil => legacy path
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if ans.Trace == nil || ans.Trace.Mode != "legacy" {
		t.Fatalf("expected legacy trace, got %+v", ans.Trace)
	}
	if len(ans.Trace.Steps) != 2 {
		t.Fatalf("expected 2 steps (retrieve, answer), got %d: %+v", len(ans.Trace.Steps), ans.Trace.Steps)
	}
	if ans.Trace.Steps[0].Action != ActionRetrieve || len(ans.Trace.Steps[0].Retrieved) != 1 {
		t.Fatalf("step0 retrieve wrong: %+v", ans.Trace.Steps[0])
	}
	if ans.Trace.Steps[0].Retrieved[0].ID != "a#0" || ans.Trace.Steps[0].Retrieved[0].Score != 0.9 {
		t.Fatalf("step0 chunk wrong: %+v", ans.Trace.Steps[0].Retrieved)
	}
	if ans.Trace.Steps[1].Action != ActionAnswer || !ans.Trace.Steps[1].GroundingOK {
		t.Fatalf("step1 answer wrong: %+v", ans.Trace.Steps[1])
	}
}

func TestOrchestratorAnswer_NoTraceByDefault(t *testing.T) {
	gen := &stubGenerator{text: "answer [source:a#0]", cites: []string{"a#0"}}
	o := &Orchestrator{
		Retriever:      stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", "fact", 1)}},
		Fusion:         PassthroughFusion{},
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Generator:      gen,
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q"}) // Trace false
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if ans.Trace != nil {
		t.Fatalf("expected nil trace by default, got %+v", ans.Trace)
	}
}
