package memory

import (
	"context"
	"strings"
	"testing"
)

// scoredChunk builds a RetrievedChunk with a score (the chunk() helper in
// agent_loop_test.go leaves Score zero).
func scoredChunk(id, content string, score float64) RetrievedChunk {
	return RetrievedChunk{Chunk: Chunk{ID: id, Content: content}, Score: score}
}

func TestAgentLoop_Trace_RetrieveThenAnswer(t *testing.T) {
	gen := &stubGenerator{text: "the answer [source:a#0]", cites: []string{"a#0"}}
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "search terms"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", "grounded fact", 0.42)}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.Trace == nil {
		t.Fatal("expected a trace when Query.Trace=true")
	}
	if ans.Trace.Mode != "agentic" {
		t.Fatalf("expected agentic mode, got %q", ans.Trace.Mode)
	}
	if len(ans.Trace.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d: %+v", len(ans.Trace.Steps), ans.Trace.Steps)
	}
	s0 := ans.Trace.Steps[0]
	if s0.Action != ActionRetrieve || s0.Query != "search terms" {
		t.Fatalf("step0 should be the retrieve, got %+v", s0)
	}
	if len(s0.Retrieved) != 1 || s0.Retrieved[0].ID != "a#0" || s0.Retrieved[0].Score != 0.42 {
		t.Fatalf("step0 retrieved chunk wrong: %+v", s0.Retrieved)
	}
	if s0.Retrieved[0].Snippet != "grounded fact" {
		t.Fatalf("step0 snippet wrong: %q", s0.Retrieved[0].Snippet)
	}
	s1 := ans.Trace.Steps[1]
	if s1.Action != ActionAnswer || !s1.GroundingOK || s1.GaveUp {
		t.Fatalf("step1 should be a grounded answer, got %+v", s1)
	}
}

func TestAgentLoop_Trace_Nil_WhenNotRequested(t *testing.T) {
	gen := &stubGenerator{text: "the answer [source:a#0]", cites: []string{"a#0"}}
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", "fact", 1)}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"}) // Trace defaults false
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.Trace != nil {
		t.Fatalf("expected nil trace when not requested, got %+v", ans.Trace)
	}
}

// Guards the spec's headline property: each retrieve step records the chunks
// THAT step fetched (the per-step delta), not the cumulative AccumulatedChunks.
// countingRetriever (agent_loop_test.go) returns a distinct chunk per call.
func TestAgentLoop_Trace_MultiRetrieve_PerStepDelta(t *testing.T) {
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "first"},
			{Type: ActionRetrieve, Query: "second"},
			{Type: ActionGiveUp, Reason: "stop"},
		}},
		Retriever: &countingRetriever{},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ans.Trace.Steps) != 3 {
		t.Fatalf("expected 3 steps (retrieve, retrieve, give_up), got %d: %+v", len(ans.Trace.Steps), ans.Trace.Steps)
	}
	s0, s1 := ans.Trace.Steps[0], ans.Trace.Steps[1]
	if len(s0.Retrieved) != 1 || len(s1.Retrieved) != 1 {
		t.Fatalf("each retrieve step must record only its own delta, got %d and %d", len(s0.Retrieved), len(s1.Retrieved))
	}
	if s0.Retrieved[0].ID == s1.Retrieved[0].ID {
		t.Fatalf("retrieve steps must hold distinct per-step chunks, both = %q (cumulative leak?)", s0.Retrieved[0].ID)
	}
}

// Guards that the trace path actually runs retrieved content through snippet():
// long, multiline content must arrive single-lined and rune-truncated.
func TestAgentLoop_Trace_SnippetTruncatedAndSingleLined(t *testing.T) {
	content := strings.Repeat("a", 60) + "\n" + strings.Repeat("b", 80) // 141 runes, one newline
	loop := &AgentLoop{
		Planner:   &stubPlanner{actions: []AgentAction{{Type: ActionRetrieve, Query: "q"}, {Type: ActionGiveUp, Reason: "stop"}}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", content, 1)}},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	snip := ans.Trace.Steps[0].Retrieved[0].Snippet
	if strings.ContainsAny(snip, "\n\r") {
		t.Fatalf("snippet must be single-lined, got %q", snip)
	}
	if !strings.HasSuffix(snip, "…") {
		t.Fatalf("over-long snippet must end with ellipsis, got %q", snip)
	}
	if n := len([]rune(snip)); n != 121 { // 120 runes + the ellipsis rune
		t.Fatalf("expected 120-rune cut + ellipsis (121 runes), got %d", n)
	}
}

func TestAgentLoop_Trace_GiveUp(t *testing.T) {
	loop := &AgentLoop{
		Planner:   &stubPlanner{actions: []AgentAction{{Type: ActionGiveUp, Reason: "not in KB"}}},
		Retriever: stubRetriever{},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.Trace == nil || len(ans.Trace.Steps) != 1 {
		t.Fatalf("expected 1-step give_up trace, got %+v", ans.Trace)
	}
	st := ans.Trace.Steps[0]
	if st.Action != ActionGiveUp || !st.GaveUp || st.Reason != "not in KB" {
		t.Fatalf("give_up step wrong: %+v", st)
	}
}

func TestAgentLoop_Trace_BudgetExhaustion(t *testing.T) {
	loop := &AgentLoop{
		Planner:   &stubPlanner{}, // always retrieve, never terminal
		Retriever: stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", "x", 1)}},
		Cfg:       AgentConfig{MaxSteps: 2},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.Trace == nil {
		t.Fatal("expected trace")
	}
	last := ans.Trace.Steps[len(ans.Trace.Steps)-1]
	if last.Action != ActionGiveUp || last.Reason != "step budget exhausted" {
		t.Fatalf("expected terminal budget give_up, got %+v", last)
	}
}

func TestAgentLoop_Trace_UngroundedAnswerGivesUp(t *testing.T) {
	gen := &stubGenerator{text: "fabricated [source:ghost]", cites: nil}
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{scoredChunk("a#0", "real fact", 1)}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q", Trace: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.GaveUp {
		t.Fatalf("ungrounded answer must give_up, got %+v", ans)
	}
	last := ans.Trace.Steps[len(ans.Trace.Steps)-1]
	if last.Action != ActionGiveUp || last.Reason != "answer ungrounded" {
		t.Fatalf("expected ungrounded give_up step, got %+v", last)
	}
}
