# Recall Reasoning Trace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in `trace` flag to `ptolemy_memory_recall` that returns a structured `steps` reasoning trace (retrieval queries, chunks with id/score/snippet, planner decisions, grounding/give-up) for both the agentic and legacy generate paths.

**Architecture:** Approach A — add `Trace bool` to `memory.Query` and `Trace *RecallTrace` to `memory.Answer`. The agent loop and the legacy `Orchestrator.Answer` path populate the trace as they run; the MCP handler surfaces it as `steps`. Trace is `nil` by default, so existing behavior is byte-for-byte unchanged.

**Tech Stack:** Go 1.26, custom in-process memory module (`internal/memory`), custom MCP server (`internal/mcp/memorytools`). Tests are standard `go test` with in-package stubs.

**Spec:** `docs/superpowers/specs/2026-06-04-recall-reasoning-trace-design.md`

---

## File Structure

- **Create** `internal/memory/trace.go` — `RecallTrace`, `TraceStep`, `TraceChunk` types + `snippet` helper. Pure data, no deps.
- **Create** `internal/memory/trace_test.go` — unit tests for `snippet`.
- **Create** `internal/memory/agent_loop_trace_test.go` — agentic-path trace tests.
- **Modify** `internal/memory/types.go` — add `Query.Trace` and `Answer.Trace`.
- **Modify** `internal/memory/agent_loop.go` — emit the agentic trace.
- **Modify** `internal/memory/orchestrator.go` — emit the legacy 2-step trace.
- **Modify** `internal/memory/orchestrator_test.go` — legacy-path trace test (or a new `orchestrator_trace_test.go`).
- **Modify** `internal/mcp/memorytools/tools.go` — `trace` schema flag + serialize `steps`.
- **Modify** `internal/mcp/memorytools/tools_test.go` — handler trace tests.
- **Modify** `docs/Architecture.md` — one-paragraph note.

---

## Task 1: Trace types + snippet helper

**Files:**
- Create: `internal/memory/trace.go`
- Test: `internal/memory/trace_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/trace_test.go`:

```go
package memory

import "testing"

func TestSnippet_ShortStringUnchanged(t *testing.T) {
	if got := snippet("hello world", 120); got != "hello world" {
		t.Fatalf("short string should be unchanged, got %q", got)
	}
}

func TestSnippet_TruncatesByRunes(t *testing.T) {
	got := snippet("abcdefghij", 4)
	if got != "abcd…" {
		t.Fatalf("expected rune-truncated with ellipsis, got %q", got)
	}
}

func TestSnippet_SingleLinesNewlines(t *testing.T) {
	if got := snippet("line one\nline two\r\nthree", 120); got != "line one line two three" {
		t.Fatalf("newlines/CRs should collapse to single spaces, got %q", got)
	}
}

func TestSnippet_CountsRunesNotBytes(t *testing.T) {
	// 5 multibyte runes, limit 5 -> unchanged (no truncation).
	if got := snippet("héllo", 5); got != "héllo" {
		t.Fatalf("rune count (not byte count) must govern, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -run TestSnippet -v`
Expected: FAIL to compile — `undefined: snippet`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/memory/trace.go`:

```go
package memory

import "strings"

// RecallTrace is the optional reasoning trace returned when Query.Trace is set.
// It is strictly additive: nil unless tracing was requested, and never affects
// the answer text or citations.
type RecallTrace struct {
	Mode  string      `json:"mode"` // "agentic" | "legacy"
	Steps []TraceStep `json:"steps"`
}

// TraceStep is one step of a recall: a planner action and its effects.
type TraceStep struct {
	Index       int          `json:"index"`
	Action      string       `json:"action"`                 // "retrieve" | "answer" | "give_up"
	Reason      string       `json:"reason,omitempty"`       // give_up reason
	Query       string       `json:"query,omitempty"`        // query a retrieve step issued
	Retrieved   []TraceChunk `json:"retrieved,omitempty"`    // per-step retrieved delta
	GaveUp      bool         `json:"gave_up,omitempty"`      // terminal give_up
	GroundingOK bool         `json:"grounding_ok,omitempty"` // terminal answer passed grounding
}

// TraceChunk is a compact view of one retrieved chunk.
type TraceChunk struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// snippet returns a single-line, rune-bounded preview of s. Newlines and
// carriage returns collapse to single spaces; if the result exceeds n runes it
// is cut to n runes and an ellipsis is appended.
func snippet(s string, n int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/memory/ -run TestSnippet -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/trace.go internal/memory/trace_test.go
git commit -m "feat(memory): RecallTrace types + snippet helper

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Agent loop emits the agentic trace

**Files:**
- Modify: `internal/memory/types.go` (add `Query.Trace`, `Answer.Trace`)
- Modify: `internal/memory/agent_loop.go`
- Test: `internal/memory/agent_loop_trace_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/memory/agent_loop_trace_test.go`:

```go
package memory

import (
	"context"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -run TestAgentLoop_Trace -v`
Expected: FAIL to compile — `Query` has no field `Trace`, `Answer` has no field `Trace`.

- [ ] **Step 3a: Add the struct fields**

In `internal/memory/types.go`, add `Trace` to `Query` (after `ProjectID`):

```go
type Query struct {
	Text      string
	K         int
	AsOf      *time.Time
	Filters   map[string]any
	SubjectID *string // recall isolation; nil = global KB only
	ProjectID *string // reserved for 6b project filtering; populated, not filtered in 6a
	Trace     bool    // when true, Answer carries a RecallTrace of the retrieval/reasoning steps
}
```

And add `Trace` to `Answer`:

```go
type Answer struct {
	Text      string
	Citations []string
	GaveUp    bool         // true when the loop produced an honest give_up (not a failure)
	Trace     *RecallTrace // non-nil only when Query.Trace was set; additive, never affects Text
}
```

- [ ] **Step 3b: Implement trace emission in the agent loop**

Replace the body of `Run`, `doRetrieve`, and `doAnswer` in `internal/memory/agent_loop.go` with the versions below, and add the three small helpers. (`doGiveUp` and `isGrounded` are unchanged.)

```go
func (a *AgentLoop) Run(ctx context.Context, q Query) (Answer, error) {
	budget := a.Cfg.MaxSteps
	if budget <= 0 {
		budget = 5
	}
	state := AgentState{Query: q.Text, Budget: budget}
	var tr *RecallTrace
	if q.Trace {
		tr = &RecallTrace{Mode: "agentic"}
	}
	for state.StepCount < state.Budget {
		action, err := a.Planner.NextAction(ctx, state)
		if err != nil {
			return Answer{}, fmt.Errorf("planner step %d: %w", state.StepCount, err)
		}
		state.Steps = append(state.Steps, action)
		switch action.Type {
		case ActionRetrieve:
			state, err = a.doRetrieve(ctx, state, q, action, tr)
			if err != nil {
				return Answer{}, err
			}
		case ActionAnswer:
			return a.doAnswer(ctx, state, q, tr)
		case ActionGiveUp:
			traceGiveUp(tr, action.Reason)
			return attachTrace(tr, a.doGiveUp(action.Reason)), nil
		default:
			return Answer{}, fmt.Errorf("planner returned unknown action %q", action.Type)
		}
		state.StepCount++
		if a.onState != nil {
			a.onState(state)
		}
	}
	traceGiveUp(tr, "step budget exhausted")
	return attachTrace(tr, a.doGiveUp("step budget exhausted")), nil
}

func (a *AgentLoop) doRetrieve(ctx context.Context, state AgentState, q Query, action AgentAction, tr *RecallTrace) (AgentState, error) {
	depth := a.Depth
	if depth <= 0 {
		depth = 20
	}
	rq := q
	rq.Text = action.Query
	chunks, err := a.Retriever.Retrieve(ctx, rq, depth)
	if err != nil {
		return state, fmt.Errorf("retrieve: %w", err)
	}
	state.AccumulatedChunks = append(state.AccumulatedChunks, chunks...)
	if tr != nil {
		tr.Steps = append(tr.Steps, retrieveStep(len(tr.Steps), action, chunks))
	}
	log.Info().Str("stage", "agent_retrieve").Int("got", len(chunks)).Int("total", len(state.AccumulatedChunks)).Msg("agent loop: retrieved")
	return state, nil
}

func (a *AgentLoop) doAnswer(ctx context.Context, state AgentState, q Query, tr *RecallTrace) (Answer, error) {
	if len(state.AccumulatedChunks) == 0 {
		traceGiveUp(tr, "no chunks")
		return attachTrace(tr, a.doGiveUp("no chunks")), nil
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = a.FinalK
	}
	chunks := state.AccumulatedChunks
	if finalK > 0 && len(chunks) > finalK {
		chunks = chunks[:finalK]
	}
	pc := a.Builder.Build(q, chunks)
	ans, err := a.Generator.Generate(ctx, q, pc)
	if err != nil {
		return Answer{}, fmt.Errorf("generate: %w", err)
	}
	if !isGrounded(ans.Text, pc.SourceIDs) {
		traceGiveUp(tr, "answer ungrounded")
		return attachTrace(tr, a.doGiveUp("answer ungrounded")), nil
	}
	if tr != nil {
		tr.Steps = append(tr.Steps, TraceStep{Index: len(tr.Steps), Action: ActionAnswer, GroundingOK: true})
	}
	return attachTrace(tr, ans), nil
}

// retrieveStep builds the trace entry for one retrieve, capturing the per-step
// delta (the chunks THIS step fetched, not the cumulative total).
func retrieveStep(idx int, action AgentAction, chunks []RetrievedChunk) TraceStep {
	tcs := make([]TraceChunk, len(chunks))
	for i, c := range chunks {
		tcs[i] = TraceChunk{ID: c.ID, Score: c.Score, Snippet: snippet(c.Content, 120)}
	}
	return TraceStep{Index: idx, Action: ActionRetrieve, Query: action.Query, Retrieved: tcs}
}

// traceGiveUp appends a terminal give_up step (no-op when tr is nil).
func traceGiveUp(tr *RecallTrace, reason string) {
	if tr == nil {
		return
	}
	tr.Steps = append(tr.Steps, TraceStep{Index: len(tr.Steps), Action: ActionGiveUp, Reason: reason, GaveUp: true})
}

// attachTrace attaches tr to ans (no-op when tr is nil) and returns ans.
func attachTrace(tr *RecallTrace, ans Answer) Answer {
	if tr != nil {
		ans.Trace = tr
	}
	return ans
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run 'TestAgentLoop' -v`
Expected: PASS — both the new `TestAgentLoop_Trace_*` tests and the pre-existing `TestAgentLoop_*` tests (regression check that the refactor preserved behavior).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/types.go internal/memory/agent_loop.go internal/memory/agent_loop_trace_test.go
git commit -m "feat(memory): agent loop emits opt-in reasoning trace

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Legacy generate path emits a 2-step trace

**Files:**
- Modify: `internal/memory/orchestrator.go` (the `Answer` method, legacy branch)
- Test: `internal/memory/orchestrator_trace_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `internal/memory/orchestrator_trace_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -run TestOrchestratorAnswer_LegacyTrace -v`
Expected: FAIL — `ans.Trace` is nil (legacy path does not build a trace yet).

- [ ] **Step 3: Implement the legacy trace**

In `internal/memory/orchestrator.go`, replace the legacy portion of `Answer` (everything after the `AgentLoop` guard) with the version below. The agentic branch is unchanged — the loop already honors `q.Trace`.

```go
func (o *Orchestrator) Answer(ctx context.Context, q Query) (Answer, error) {
	if o.AgentLoop != nil {
		return o.AgentLoop.Run(ctx, q)
	}
	depth := o.Depth
	if depth <= 0 {
		depth = 20
	}
	asOf := time.Now().UTC()
	if q.AsOf != nil {
		asOf = *q.AsOf
	}
	local := q
	local.AsOf = &asOf

	candidates, err := o.Retriever.Retrieve(ctx, local, depth)
	if err != nil {
		return Answer{}, fmt.Errorf("retrieve: %w", err)
	}
	if o.Store != nil && len(candidates) > 0 {
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		if err := o.Store.Reinforce(ctx, ids); err != nil {
			log.Warn().Err(err).Msg("reinforce failed; serving answer anyway")
		}
	}
	finalK := q.K
	if finalK <= 0 {
		finalK = o.FinalK
	}
	fused := o.Fusion.Fuse([][]RetrievedChunk{candidates}, finalK)
	prompt := o.ContextBuilder.Build(q, fused)
	ans, err := o.Generator.Generate(ctx, q, prompt)
	if err != nil {
		return Answer{}, err
	}
	if q.Trace {
		ans.Trace = &RecallTrace{
			Mode: "legacy",
			Steps: []TraceStep{
				retrieveStep(0, AgentAction{Query: q.Text}, fused),
				{Index: 1, Action: ActionAnswer, GroundingOK: isGrounded(ans.Text, prompt.SourceIDs)},
			},
		}
	}
	return ans, nil
}
```

Note: the legacy `Generate` historically returned its result directly; capturing it in `ans` first lets us attach the trace without changing the returned value when `q.Trace` is false.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run 'TestOrchestratorAnswer' -v`
Expected: PASS (both new tests).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/orchestrator_trace_test.go
git commit -m "feat(memory): legacy generate path emits 2-step recall trace

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: MCP recall tool exposes `trace` and serializes `steps`

**Files:**
- Modify: `internal/mcp/memorytools/tools.go` (recall schema + `handleRecall`)
- Test: `internal/mcp/memorytools/tools_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/mcp/memorytools/tools_test.go`:

```go
func TestHandleRecallTraceReturnsSteps(t *testing.T) {
	rec := &fakeRecaller{answer: memory.Answer{
		Text:      "prose [source:a#0]",
		Citations: []string{"a#0"},
		Trace: &memory.RecallTrace{
			Mode: "agentic",
			Steps: []memory.TraceStep{
				{Index: 0, Action: "retrieve", Query: "terms", Retrieved: []memory.TraceChunk{{ID: "a#0", Score: 0.5, Snippet: "fact"}}},
				{Index: 1, Action: "answer", GroundingOK: true},
			},
		},
	}}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	res, _, err := h("ptolemy_memory_recall", map[string]any{"query": "q", "trace": true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// trace=true must route through Answer (the generate path) and set Query.Trace.
	if rec.answerCalls != 1 || rec.recallCalls != 0 {
		t.Fatalf("trace=true must use Answer, got recall=%d answer=%d", rec.recallCalls, rec.answerCalls)
	}
	if !rec.gotQuery.Trace {
		t.Fatal("expected Query.Trace=true passed through")
	}
	var out struct {
		Text  string `json:"text"`
		Mode  string `json:"mode"`
		Steps []struct {
			Action    string `json:"action"`
			Query     string `json:"query"`
			Retrieved []struct {
				ID      string  `json:"id"`
				Score   float64 `json:"score"`
				Snippet string  `json:"snippet"`
			} `json:"retrieved"`
			GroundingOK bool `json:"grounding_ok"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out.Mode != "agentic" || len(out.Steps) != 2 {
		t.Fatalf("unexpected steps payload: %#v", out)
	}
	if out.Steps[0].Action != "retrieve" || len(out.Steps[0].Retrieved) != 1 || out.Steps[0].Retrieved[0].ID != "a#0" {
		t.Fatalf("step0 wrong: %#v", out.Steps[0])
	}
	if !out.Steps[1].GroundingOK {
		t.Fatalf("step1 grounding flag missing: %#v", out.Steps[1])
	}
}

func TestHandleRecallNoTraceOmitsSteps(t *testing.T) {
	rec := &fakeRecaller{recall: memory.RecallResult{Context: "ctx", SourceIDs: []string{"a#0"}}}
	h := NewHandler(Deps{Recall: rec, Subject: "luan", Project: "ptolemy"})

	res, _, err := h("ptolemy_memory_recall", map[string]any{"query": "q"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Regression: default path unchanged — retrieval-only, no steps key.
	if rec.recallCalls != 1 || rec.answerCalls != 0 {
		t.Fatalf("default must stay retrieval-only, got recall=%d answer=%d", rec.recallCalls, rec.answerCalls)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(textOf(t, res)), &raw); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, ok := raw["steps"]; ok {
		t.Fatalf("steps must be absent without trace=true, got %#v", raw)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/memorytools/ -run TestHandleRecallTrace -v`
Expected: FAIL — `trace=true` currently falls through to the retrieval-only path; `answerCalls` is 0 and there is no `steps`/`mode` in the payload.

- [ ] **Step 3: Implement the schema flag + handler branch**

In `internal/mcp/memorytools/tools.go`, add the `trace` property to the recall tool schema (inside the `properties` map of `ptolemy_memory_recall`, after `generate`):

```go
					"generate":   map[string]any{"type": "boolean", "description": "Synthesize a prose answer via the LLM (slower). Default false returns retrieval-only context."},
					"trace":      map[string]any{"type": "boolean", "description": "Return a step-by-step reasoning trace (retrieval queries, retrieved chunks with id/score/snippet, planner decisions, grounding). Implies generate=true. Default false."},
```

Then update `handleRecall` so the generate/answer path fires when EITHER `generate` or `trace` is set, threads `q.Trace`, and serializes the trace:

```go
	gen, _ := args["generate"].(bool)
	trace, _ := args["trace"].(bool)
	q.Trace = trace
	if gen || trace {
		ans, err := d.Recall.Answer(context.Background(), q)
		if err != nil {
			return nil, true, err
		}
		payload := map[string]any{"text": ans.Text, "citations": ans.Citations}
		if ans.Trace != nil {
			payload["mode"] = ans.Trace.Mode
			payload["steps"] = ans.Trace.Steps
		}
		return jsonResult(payload), true, nil
	}
	res, err := d.Recall.Recall(context.Background(), q)
	if err != nil {
		return nil, true, err
	}
	return jsonResult(map[string]any{"text": res.Context, "citations": res.SourceIDs}), true, nil
```

This replaces the existing `if gen, _ := args["generate"].(bool); gen { ... }` block plus the trailing `Recall` block (lines ~122–133). The `q.Trace = trace` assignment must come before the `Answer` call so the flag reaches the orchestrator.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mcp/memorytools/ -v`
Expected: PASS — the two new tests plus all pre-existing memorytools tests (regression: `TestHandleRecall`, `TestHandleRecallGenerateUsesLLM`, etc. unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/memorytools/tools.go internal/mcp/memorytools/tools_test.go
git commit -m "feat(mcp): recall tool exposes opt-in trace returning reasoning steps

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Architecture note + full verification

**Files:**
- Modify: `docs/Architecture.md`

- [ ] **Step 1: Add the doc note**

Find the memory/recall section of `docs/Architecture.md` (search for "recall" or "memory"). Append this paragraph:

```markdown
### Recall reasoning trace

`ptolemy_memory_recall` accepts an opt-in `trace` boolean (implies `generate`).
When set, the result carries `mode` (`agentic`|`legacy`) and a `steps` array: one
entry per recall step with the planner action, the query it issued, the chunks it
retrieved (id, score, ~120-rune snippet), and the terminal outcome (grounding
result or give-up reason). The trace is built in-memory from data the loop already
holds, is `nil`/absent by default, and never alters the answer text or citations.
Implemented via `RecallTrace`/`TraceStep`/`TraceChunk` in `internal/memory/trace.go`,
emitted by `AgentLoop.Run` (agentic) and `Orchestrator.Answer` (legacy). No live
streaming or MCP notifications — the trace is returned in the single tool result.
```

- [ ] **Step 2: Run the full memory + mcp suites**

Run: `go test ./internal/memory/... ./internal/mcp/...`
Expected: PASS (no failures). If a DB-backed test is skipped without `DATABASE_URL`, that is expected — the new tests are all in-memory and must pass regardless.

- [ ] **Step 3: Build to confirm the binaries compile**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add docs/Architecture.md
git commit -m "docs(memory): note opt-in recall reasoning trace in Architecture

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Opt-in `trace` flag, default unchanged → Task 4 (schema + handler), Task 2/3 (`nil` by default tests).
- `steps` with action/reason/query/chunks(id,score,snippet)/grounding → Task 1 (types), Task 2/3 (emit), Task 4 (serialize).
- Agentic path coverage → Task 2. Legacy path coverage → Task 3.
- `trace` implies generate → Task 4 (`gen || trace`).
- Additive / never breaks the answer → `attachTrace`/nil-guards (Task 2), default-nil tests (Task 2/3/4).
- Snippet ~120 runes, single-line → Task 1 (`snippet`).
- Architecture note → Task 5.

**Placeholder scan:** none — every code step shows complete code; every run step shows the command and expected result.

**Type consistency:** `RecallTrace{Mode,Steps}`, `TraceStep{Index,Action,Reason,Query,Retrieved,GaveUp,GroundingOK}`, `TraceChunk{ID,Score,Snippet}`, `Query.Trace`, `Answer.Trace`, helpers `snippet`/`retrieveStep`/`traceGiveUp`/`attachTrace` are used identically across Tasks 1–4. JSON keys (`mode`,`steps`,`action`,`query`,`retrieved`,`id`,`score`,`snippet`,`grounding_ok`) match the struct tags in Task 1 and the handler test in Task 4.
