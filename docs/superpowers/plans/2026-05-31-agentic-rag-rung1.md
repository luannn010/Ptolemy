# Agentic RAG (Rung 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `internal/memory` from a fixed pipeline (`Orchestrator.Answer`) to an agent loop where the LLM chooses `retrieve` / `answer` / `give_up`, bounded by a hard step budget, with a grounding check that turns ungrounded answers into honest `give_up` — all behind a default-off feature flag.

**Architecture:** A new `Planner` (grammar-constrained BRAIN call, reusing the extractor's GBNF + validator-chain pattern) drives an `AgentLoop` that invokes the *existing* `Retriever`, `ContextBuilder`, and `Generator` as steps. `Orchestrator.Answer` delegates to the loop only when `AGENT_LOOP_ENABLED=true`; otherwise the legacy path is byte-for-byte unchanged. A new `-agent` eval mode scores give-up correctness + grounding (the generation path the retrieval-only harness can't see), while retrieval recall@5 stays a no-regression guard.

**Tech Stack:** Go 1.25, `internal/memory` package, llama.cpp BRAIN (`:1090`, Qwen3.5-4B, `enable_thinking=false`), GBNF grammar via `//go:embed`, Postgres+pgvector (unchanged), `net/http/httptest` for unit tests.

---

## Discovery Summary (answers to prompt §0)

**Q1 — What does `Orchestrator.Answer` do today, stage by stage?**
[orchestrator.go:187-221](../../../internal/memory/orchestrator.go#L187-L221). Single pass, no LLM in the control loop:
1. Default `depth` (20) and resolve `AsOf` once (non-nil downstream).
2. `Retriever.Retrieve(ctx, local, depth)` → `[]RetrievedChunk` (production retriever is `HybridRetriever`: BM25 `@@@` + vector `<=>` fused with RRF C=60 inside one SQL query, plus recency + project-decay terms).
3. If `Store != nil` and candidates exist: `Store.Reinforce(ids)` (best-effort; logged-and-ignored on error).
4. `finalK` = `q.K` or `o.FinalK`.
5. `Fusion.Fuse([][]RetrievedChunk{candidates}, finalK)` — production `Fusion` is `PassthroughFusion` (truncate to k).
6. `ContextBuilder.Build(q, fused)` → `PromptContext{System,User,SourceIDs}` — production builder is `MMRContextBuilder` (synthesis-first ordering, MMR-diverse atoms, rune budget 6000).
7. `Generator.Generate(ctx, q, prompt)` → `Answer{Text, Citations}`. The generator **already drops hallucinated citations** ([generator.go:114-130](../../../internal/memory/generator.go#L114-L130)): only `[source:id]` matches whose id is in `pc.SourceIDs` survive. That is a per-citation filter, not an answer-level grounding gate — the new grounding check builds on it.

Dependencies are all interfaces ([orchestrator.go:15-25](../../../internal/memory/orchestrator.go#L15-L25)), wired in `NewModule` ([module.go:18-48](../../../internal/memory/module.go#L18-L48)). `Recall` ([orchestrator.go:120-185](../../../internal/memory/orchestrator.go#L120-L185)) is the retrieval-only fast path (no Generator) used by hooks/MCP.

**Q2 — BRAIN latency (post-GPU offload).** `time curl` against `:1090` with `enable_thinking=false`: **cold ~5.6s, warm ~0.2s** (Qwen3.5-4B-Q4_K_M). Implication: a 5-step loop is 5 sequential BRAIN calls; the step budget bounds latency as much as safety. The planner uses `enable_thinking=false` like every other memory call.

**Q3 — Eval baseline.** `make eval-memory`: **mean recall@5 = 1.000 over 30 questions** (paraphrase 1.000/12, exact_token 1.000/8, fresh_vs_stale 1.000/8; the 2 negative questions have `Expected=[]` and are excluded from the mean). **Critical:** `eval.RunRetrieval` is *retrieval-only and deliberately never calls the Generator* ([eval.go:124-150](../../../internal/memory/eval/eval.go#L124-L150)). So the loop's `answer`/`give_up`/grounding logic is invisible to recall@5, and a 1.000 baseline can only regress. **Resolution (user-approved):** add an `-agent` eval mode that runs the loop end-to-end and scores give-up correctness + grounding; keep retrieval recall@5 as a no-regression guard.

**Q4 — Interfaces already clean vs. need lifting.** Already interfaces and directly usable as loop tools: `Retriever`, `Fusion`, `ContextBuilder`, `Generator`, `ChatClient` ([extractor.go:23-25](../../../internal/memory/extractor.go#L23-L25)). The extractor pattern to copy: embedded GBNF (`//go:embed grammar/atom.gbnf`), struct-drift test ([grammar_test.go:31-48](../../../internal/memory/grammar_test.go#L31-L48)), deterministic `ValidatorChain` ([validators.go:121-171](../../../internal/memory/validators.go#L121-L171)). Nothing needs lifting — the loop is a new *caller*, not a re-plumbing. New config knobs follow the `intEnv`/`boolEnv` pattern ([config.go:164-210](../../../internal/memory/config.go#L164-L210)).

---

## File Structure

- **Create** `internal/memory/agent_types.go` — `AgentAction`, `AgentState`, `Planner` interface, `AgentConfig`. One file: small, cohesive, the loop's vocabulary.
- **Create** `internal/memory/grammar/action.gbnf` — GBNF constraining planner output to the `AgentAction` schema. Sibling of `atom.gbnf`.
- **Create** `internal/memory/prompts/plan_v1.txt` — planner system prompt. Sibling of `extract_v1.txt`.
- **Create** `internal/memory/planner.go` — `BrainPlanner` (a `ChatClient` wrapper) + action validator chain. Mirrors `extractor.go` + the validator half of `validators.go`.
- **Create** `internal/memory/agent_loop.go` — `AgentLoop` with `Run`, `doRetrieve`, `doAnswer` (incl. grounding check), `doGiveUp`.
- **Modify** `internal/memory/config.go` — add `Agent AgentLoopConfig` (flag + max steps) to `MemoryConfig` and `LoadConfig`.
- **Modify** `internal/memory/orchestrator.go` — `Answer` delegates to a wired `*AgentLoop` when the flag is on; legacy path otherwise.
- **Modify** `internal/memory/module.go` — `NewModule` builds the planner + loop, wires them onto the Orchestrator, reads the flag.
- **Create** `internal/memory/eval/agent.go` — `RunAgentEval`: per-question give-up/grounding scoring + summary.
- **Modify** `internal/cli/memory/eval.go` — add `-agent` flag → call `RunAgentEval`.
- **Modify** `Makefile` — add `eval-memory-agent` target.
- **Modify** `docs/Architecture.md` — one-paragraph note per the package-port DoD.

Test files mirror each source file (`*_test.go`), per the existing convention.

---

## Task 1: Action & state types

**Files:**
- Create: `internal/memory/agent_types.go`
- Test: `internal/memory/agent_types_test.go`

- [ ] **Step 1: Write the failing test** — `internal/memory/agent_types_test.go`

```go
package memory

import "testing"

func TestAgentAction_TerminalAndRetrieveTypes(t *testing.T) {
	for _, typ := range []string{ActionRetrieve, ActionAnswer, ActionGiveUp} {
		if typ == "" {
			t.Fatalf("action type constant is empty")
		}
	}
	if ActionRetrieve == ActionAnswer || ActionAnswer == ActionGiveUp {
		t.Fatal("action type constants must be distinct")
	}
}

func TestAgentState_BudgetRemaining(t *testing.T) {
	s := AgentState{Budget: 5, StepCount: 2}
	if got := s.budgetRemaining(); got != 3 {
		t.Fatalf("budgetRemaining = %d, want 3", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'TestAgentAction_TerminalAndRetrieveTypes|TestAgentState_BudgetRemaining'`
  Expected: FAIL (undefined: `ActionRetrieve`, `AgentState`).

- [ ] **Step 3: Write minimal implementation** — `internal/memory/agent_types.go`

```go
package memory

import "context"

// Action type tags. Terminal actions are ActionAnswer and ActionGiveUp; a
// planner that returns neither and exhausts the budget is forced to give_up.
const (
	ActionRetrieve = "retrieve"
	ActionAnswer   = "answer"
	ActionGiveUp   = "give_up"
)

// AgentAction is the grammar-constrained output of the planner.
type AgentAction struct {
	Type   string `json:"type"`
	Query  string `json:"query"`  // for "retrieve"
	Reason string `json:"reason"` // for "give_up"
}

// AgentState carries everything the planner needs to decide the next step.
type AgentState struct {
	Query             string
	AccumulatedChunks []RetrievedChunk
	Steps             []AgentAction // history of actions taken
	StepCount         int
	Budget            int // AGENT_MAX_STEPS
}

func (s AgentState) budgetRemaining() int { return s.Budget - s.StepCount }

// Planner returns the next AgentAction given the current state.
type Planner interface {
	NextAction(ctx context.Context, state AgentState) (AgentAction, error)
}

// AgentConfig holds the loop's runtime knobs (sourced from MemoryConfig.Agent).
type AgentConfig struct {
	MaxSteps int
}
```

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'TestAgentAction_TerminalAndRetrieveTypes|TestAgentState_BudgetRemaining'`
  Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/agent_types.go internal/memory/agent_types_test.go
git commit -m "feat(memory): add agent loop action and state types"
```

---

## Task 2: GBNF grammar for `AgentAction` (+ struct-drift guard)

**Files:**
- Create: `internal/memory/grammar/action.gbnf`
- Test: `internal/memory/grammar_test.go` (add cases; do not edit existing)

- [ ] **Step 1: Write the failing test** — append to `internal/memory/grammar_test.go`

```go
func TestActionGrammarFile_ValidGBNF(t *testing.T) {
	b, err := os.ReadFile("grammar/action.gbnf")
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	s := strings.ReplaceAll(string(b), `\"`, `"`)
	for _, required := range []string{"root ::=", `"type"`, "actiontype ::=", `"retrieve"`, `"answer"`, `"give_up"`} {
		if !strings.Contains(s, required) {
			t.Fatalf("action grammar missing %q", required)
		}
	}
}

func TestActionGrammarMatchesAgentActionStruct(t *testing.T) {
	b, err := os.ReadFile("grammar/action.gbnf")
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	s := strings.ReplaceAll(string(b), `\"`, `"`)
	rt := reflect.TypeOf(AgentAction{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" {
			continue
		}
		if !strings.Contains(s, `"`+tag+`"`) {
			t.Fatalf("action grammar missing field %q from AgentAction json tag", tag)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'ActionGrammar'`
  Expected: FAIL (`open grammar/action.gbnf: no such file`).

- [ ] **Step 3: Write minimal implementation** — `internal/memory/grammar/action.gbnf`

```gbnf
root ::= ws object ws
object ::= "{" ws
           "\"type\"" ws ":" ws actiontype ws "," ws
           "\"query\"" ws ":" ws string ws "," ws
           "\"reason\"" ws ":" ws string
           ws "}"
actiontype ::= "\"retrieve\"" | "\"answer\"" | "\"give_up\""

string ::= "\"" chars "\""
chars ::= char*
char ::= [^"\\\x00-\x1F] | escape
escape ::= "\\" (["\\/bfnrt] | unicode)
unicode ::= "u" hex hex hex hex
hex ::= [0-9a-fA-F]
ws ::= [ \t\n\r]*
```

> Note: the grammar requires all three fields always present (the model emits `"query":""` / `"reason":""` when unused). This keeps the grammar simple and the struct-drift test honest; validators (Task 3) enforce the per-type semantics.

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'ActionGrammar'`
  Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/grammar/action.gbnf internal/memory/grammar_test.go
git commit -m "feat(memory): add GBNF grammar for AgentAction with drift guard"
```

---

## Task 3: Action validator chain

**Files:**
- Create: `internal/memory/planner.go` (validators portion first)
- Test: `internal/memory/planner_test.go`

- [ ] **Step 1: Write the failing test** — `internal/memory/planner_test.go`

```go
package memory

import "testing"

func TestActionValidator_RejectsUnknownType(t *testing.T) {
	err := validateAction(AgentAction{Type: "frobnicate"})
	if err == nil {
		t.Fatal("expected unknown action type to be rejected")
	}
}

func TestActionValidator_RejectsRetrieveWithEmptyQuery(t *testing.T) {
	if err := validateAction(AgentAction{Type: ActionRetrieve, Query: "  "}); err == nil {
		t.Fatal("expected empty retrieve query to be rejected")
	}
}

func TestActionValidator_RejectsGiveUpWithEmptyReason(t *testing.T) {
	if err := validateAction(AgentAction{Type: ActionGiveUp, Reason: ""}); err == nil {
		t.Fatal("expected empty give_up reason to be rejected")
	}
}

func TestActionValidator_AcceptsValidAnswer(t *testing.T) {
	if err := validateAction(AgentAction{Type: ActionAnswer}); err != nil {
		t.Fatalf("answer action should validate: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'ActionValidator'`
  Expected: FAIL (undefined: `validateAction`).

- [ ] **Step 3: Write minimal implementation** — `internal/memory/planner.go`

```go
package memory

import (
	"fmt"
	"strings"
)

// validateAction enforces per-type semantics the grammar can't: known type,
// non-empty retrieve query, non-empty give_up reason. The grammar already
// guarantees the JSON shape and a known type literal; this is defense in depth
// plus the empty-field rules.
func validateAction(a AgentAction) error {
	switch a.Type {
	case ActionRetrieve:
		if strings.TrimSpace(a.Query) == "" {
			return fmt.Errorf("retrieve action requires a non-empty query")
		}
	case ActionAnswer:
		// no required fields
	case ActionGiveUp:
		if strings.TrimSpace(a.Reason) == "" {
			return fmt.Errorf("give_up action requires a non-empty reason")
		}
	default:
		return fmt.Errorf("unknown action type %q", a.Type)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'ActionValidator'`
  Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/planner.go internal/memory/planner_test.go
git commit -m "feat(memory): add agent action validator chain"
```

---

## Task 4: `BrainPlanner` (grammar-constrained LLM call)

**Files:**
- Create: `internal/memory/prompts/plan_v1.txt`
- Modify: `internal/memory/planner.go`
- Test: `internal/memory/planner_test.go`

- [ ] **Step 1: Write the failing test** — append to `internal/memory/planner_test.go`

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrainPlanner_SendsGrammarAndParsesAction(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"type":"retrieve","query":"rrf constant","reason":""}`}},
			},
		})
	}))
	defer srv.Close()

	p := NewBrainPlanner(NewOpenAIGenerator(srv.URL, "test-model", ""))
	act, err := p.NextAction(context.Background(), AgentState{Query: "what is the rrf constant", Budget: 5})
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}
	if act.Type != ActionRetrieve || act.Query != "rrf constant" {
		t.Fatalf("unexpected action: %+v", act)
	}
	if grammar, _ := got["grammar"].(string); strings.TrimSpace(grammar) == "" {
		t.Fatal("expected grammar field in request body")
	}
}

func TestBrainPlanner_RejectsInvalidAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"type":"retrieve","query":"","reason":""}`}},
			},
		})
	}))
	defer srv.Close()
	p := NewBrainPlanner(NewOpenAIGenerator(srv.URL, "test-model", ""))
	if _, err := p.NextAction(context.Background(), AgentState{Query: "q", Budget: 5}); err == nil {
		t.Fatal("expected validation error for empty retrieve query")
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'BrainPlanner'`
  Expected: FAIL (undefined: `NewBrainPlanner`).

- [ ] **Step 3: Write the prompt** — `internal/memory/prompts/plan_v1.txt`

```text
You are the planner for a retrieval agent answering a question from a knowledge base.

Decide the SINGLE next action. Return ONLY a JSON object:
{"type": "retrieve"|"answer"|"give_up", "query": string, "reason": string}

Actions:
- "retrieve": fetch more context. Put a focused search query in "query" (leave "reason" ""). Use this when you have no chunks yet, or the chunks so far do not contain enough to answer.
- "answer": you have enough grounded context to answer. Leave "query" and "reason" "".
- "give_up": the knowledge base does not contain the answer. Put a one-sentence explanation in "reason" (leave "query" ""). Giving up honestly is correct when the context does not support an answer — do NOT invent facts.

Rules:
- Return exactly one action, as JSON only, no prose.
- Prefer "answer" once the retrieved context clearly supports a grounded response.
- Prefer "give_up" over guessing when the context is irrelevant or empty.

The next message describes the question, the chunks retrieved so far, and the step count.
```

- [ ] **Step 4: Implement `BrainPlanner`** — append to `internal/memory/planner.go`

```go
import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed prompts/plan_v1.txt
var planPromptTemplate string

//go:embed grammar/action.gbnf
var actionGrammar string

// PlannerVersion stamps the planner prompt for auditability/re-runs.
const PlannerVersion = "plan_v1"

// BrainPlanner is a thin grammar-constrained wrapper over a ChatClient. It
// mirrors Extractor: embedded prompt + embedded GBNF + a validator pass.
type BrainPlanner struct {
	Client  ChatClient
	Grammar string
}

func NewBrainPlanner(c ChatClient) *BrainPlanner {
	return &BrainPlanner{Client: c, Grammar: strings.TrimSpace(actionGrammar)}
}

func (p *BrainPlanner) NextAction(ctx context.Context, state AgentState) (AgentAction, error) {
	user := renderPlannerState(state)
	raw, err := p.Client.Complete(ctx, planPromptTemplate, user, CompleteOptions{Grammar: p.Grammar})
	if err != nil {
		return AgentAction{}, fmt.Errorf("planner llm: %w", err)
	}
	var act AgentAction
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &act); err != nil {
		return AgentAction{}, fmt.Errorf("planner parse: %w (raw=%q)", err, raw)
	}
	if err := validateAction(act); err != nil {
		return AgentAction{}, fmt.Errorf("planner action invalid: %w", err)
	}
	return act, nil
}

// renderPlannerState formats the state into the planner's user message. Chunk
// bodies are previewed (not full) to keep the prompt bounded.
func renderPlannerState(state AgentState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\n", state.Query)
	fmt.Fprintf(&b, "Step %d of %d.\n\n", state.StepCount+1, state.Budget)
	if len(state.AccumulatedChunks) == 0 {
		b.WriteString("Chunks retrieved so far: none.\n")
	} else {
		fmt.Fprintf(&b, "Chunks retrieved so far (%d):\n", len(state.AccumulatedChunks))
		for i, c := range state.AccumulatedChunks {
			fmt.Fprintf(&b, "[%d] %s\n", i+1, preview(c.Content, 160))
		}
	}
	return b.String()
}
```

- [ ] **Step 5: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'BrainPlanner'`
  Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/planner.go internal/memory/planner_test.go internal/memory/prompts/plan_v1.txt
git commit -m "feat(memory): add BrainPlanner with grammar-constrained action call"
```

---

## Task 5: Agent loop — budget, retrieve, give_up

**Files:**
- Create: `internal/memory/agent_loop.go`
- Test: `internal/memory/agent_loop_test.go`

- [ ] **Step 1: Write the failing tests** — `internal/memory/agent_loop_test.go`

```go
package memory

import (
	"context"
	"testing"
)

// stubPlanner returns a scripted sequence of actions, one per call.
type stubPlanner struct {
	actions []AgentAction
	i       int
}

func (s *stubPlanner) NextAction(_ context.Context, _ AgentState) (AgentAction, error) {
	if s.i >= len(s.actions) {
		// never-terminal: keep returning retrieve to test budget exhaustion
		return AgentAction{Type: ActionRetrieve, Query: "more"}, nil
	}
	a := s.actions[s.i]
	s.i++
	return a, nil
}

// stubRetriever returns a fixed batch of chunks regardless of query.
type stubRetriever struct{ chunks []RetrievedChunk }

func (s stubRetriever) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	return s.chunks, nil
}

func chunk(id, content string) RetrievedChunk {
	return RetrievedChunk{Chunk: Chunk{ID: id, Content: content}}
}

func TestAgentLoop_BudgetExhaustionForcesGiveUp(t *testing.T) {
	loop := &AgentLoop{
		Planner:   &stubPlanner{}, // always returns retrieve
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "x")}},
		Cfg:       AgentConfig{MaxSteps: 3},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.Text == "" || !ans.gaveUp() {
		t.Fatalf("expected give_up at budget exhaustion, got %+v", ans)
	}
}

func TestAgentLoop_GiveUpActionShortCircuits(t *testing.T) {
	loop := &AgentLoop{
		Planner:   &stubPlanner{actions: []AgentAction{{Type: ActionGiveUp, Reason: "not in KB"}}},
		Retriever: stubRetriever{},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.gaveUp() {
		t.Fatalf("expected give_up, got %+v", ans)
	}
}

func TestAgentLoop_TwoRetrieves_ChunksAccumulate(t *testing.T) {
	var seen int
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionRetrieve, Query: "b"},
			{Type: ActionGiveUp, Reason: "stop"},
		}},
		Retriever: retrieverFunc(func() []RetrievedChunk {
			seen++
			return []RetrievedChunk{chunk("c"+string(rune('0'+seen))+"#0", "x")}
		}),
		Cfg: AgentConfig{MaxSteps: 5},
		// capture final state via hook below
	}
	loop.onState = func(s AgentState) {
		if s.StepCount == 2 && len(s.AccumulatedChunks) != 2 {
			t.Fatalf("expected 2 accumulated chunks after 2 retrieves, got %d", len(s.AccumulatedChunks))
		}
	}
	if _, err := loop.Run(context.Background(), Query{Text: "q"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

type retrieverFunc func() []RetrievedChunk

func (f retrieverFunc) Retrieve(_ context.Context, _ Query, _ int) ([]RetrievedChunk, error) {
	return f(), nil
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'TestAgentLoop_'`
  Expected: FAIL (undefined: `AgentLoop`, `Answer.gaveUp`).

- [ ] **Step 3: Write minimal implementation** — `internal/memory/agent_loop.go`

```go
package memory

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

// giveUpMarker is stored in Answer.Citations? No — keep Answer's shape. We add a
// dedicated flag method via a sentinel prefix on Text would be fragile; instead
// the loop returns an Answer whose GaveUp field is set. Extend Answer minimally.

// AgentLoop runs the LLM in the control loop, invoking the existing retriever,
// context builder, and generator as steps until a terminal action or the step
// budget is exhausted (which forces give_up).
type AgentLoop struct {
	Retriever Retriever
	Builder   ContextBuilder
	Generator Generator
	Planner   Planner
	Cfg       AgentConfig
	Depth     int
	FinalK    int

	// onState is a test hook invoked after each step's state update (nil in prod).
	onState func(AgentState)
}

func (a *AgentLoop) Run(ctx context.Context, q Query) (Answer, error) {
	budget := a.Cfg.MaxSteps
	if budget <= 0 {
		budget = 5
	}
	state := AgentState{Query: q.Text, Budget: budget}
	for state.StepCount < state.Budget {
		action, err := a.Planner.NextAction(ctx, state)
		if err != nil {
			return Answer{}, fmt.Errorf("planner step %d: %w", state.StepCount, err)
		}
		state.Steps = append(state.Steps, action)
		switch action.Type {
		case ActionRetrieve:
			state, err = a.doRetrieve(ctx, state, q, action)
			if err != nil {
				return Answer{}, err
			}
		case ActionAnswer:
			return a.doAnswer(ctx, state, q)
		case ActionGiveUp:
			return a.doGiveUp(action.Reason), nil
		default:
			return Answer{}, fmt.Errorf("planner returned unknown action %q", action.Type)
		}
		state.StepCount++
		if a.onState != nil {
			a.onState(state)
		}
	}
	return a.doGiveUp("step budget exhausted"), nil
}

func (a *AgentLoop) doRetrieve(ctx context.Context, state AgentState, q Query, action AgentAction) (AgentState, error) {
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
	log.Info().Str("stage", "agent_retrieve").Int("got", len(chunks)).Int("total", len(state.AccumulatedChunks)).Msg("agent loop: retrieved")
	return state, nil
}

func (a *AgentLoop) doGiveUp(reason string) Answer {
	log.Info().Str("stage", "agent_give_up").Str("reason", reason).Msg("agent loop: give up")
	return Answer{Text: "I don't know based on what I found: " + reason, GaveUp: true}
}
```

  Also extend `Answer` in `types.go`:

```go
type Answer struct {
	Text      string
	Citations []string
	GaveUp    bool // true when the loop produced an honest give_up (not a failure)
}
```

  And add the test helper method in `agent_loop.go`:

```go
func (an Answer) gaveUp() bool { return an.GaveUp }
```

> `doAnswer` is intentionally NOT implemented yet — Task 6 adds it via TDD. To compile, add a temporary stub that the Task 6 test will replace:

```go
func (a *AgentLoop) doAnswer(ctx context.Context, state AgentState, q Query) (Answer, error) {
	return Answer{}, fmt.Errorf("doAnswer not implemented")
}
```

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'TestAgentLoop_'`
  Expected: PASS (budget, give_up, accumulation tests; `doAnswer` untested here).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/agent_loop.go internal/memory/agent_loop_test.go internal/memory/types.go
git commit -m "feat(memory): add agent loop with step budget and give_up"
```

---

## Task 6: `doAnswer` + grounding check

**Files:**
- Modify: `internal/memory/agent_loop.go`
- Test: `internal/memory/agent_loop_test.go`

- [ ] **Step 1: Write the failing tests** — append to `internal/memory/agent_loop_test.go`

```go
// stubGenerator returns a fixed answer text + citations.
type stubGenerator struct {
	text  string
	cites []string
	calls int
}

func (g *stubGenerator) Generate(_ context.Context, _ Query, _ PromptContext) (Answer, error) {
	g.calls++
	return Answer{Text: g.text, Citations: g.cites}, nil
}

func TestAgentLoop_AnswerWithoutChunks_GivesUp(t *testing.T) {
	gen := &stubGenerator{text: "anything"}
	loop := &AgentLoop{
		Planner:   &stubPlanner{actions: []AgentAction{{Type: ActionAnswer}}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.gaveUp() {
		t.Fatalf("answer with no chunks must give_up, got %+v", ans)
	}
	if gen.calls != 0 {
		t.Fatalf("generator must not be called with no chunks, calls=%d", gen.calls)
	}
}

func TestGroundingCheck_CitationNotInChunks_FailsGrounding(t *testing.T) {
	// answer cites [source:ghost] which is not in the chunk set.
	if isGrounded("text [source:ghost] more", []string{"real"}) {
		t.Fatal("citation not in chunks must fail grounding")
	}
	if !isGrounded("text [source:real] more", []string{"real"}) {
		t.Fatal("citation present in chunks must pass grounding")
	}
	if isGrounded("no citations at all", []string{"real"}) {
		t.Fatal("an answer with zero citations is ungrounded")
	}
}

func TestAgentLoop_RetrieveThenAnswer_HappyPath(t *testing.T) {
	gen := &stubGenerator{text: "the answer [source:a#0]", cites: []string{"a#0"}}
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "grounded fact")}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.gaveUp() {
		t.Fatalf("happy path must not give_up: %+v", ans)
	}
	if gen.calls != 1 {
		t.Fatalf("generator should be called once, calls=%d", gen.calls)
	}
	if len(ans.Citations) != 1 || ans.Citations[0] != "a#0" {
		t.Fatalf("expected citation a#0, got %v", ans.Citations)
	}
}

func TestAgentLoop_UngroundedAnswer_BecomesGiveUp(t *testing.T) {
	gen := &stubGenerator{text: "fabricated [source:ghost]", cites: nil} // generator drops ghost → no citations
	loop := &AgentLoop{
		Planner: &stubPlanner{actions: []AgentAction{
			{Type: ActionRetrieve, Query: "a"},
			{Type: ActionAnswer},
		}},
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "real fact")}},
		Generator: gen,
		Builder:   BudgetContextBuilder{MaxRunes: 6000},
		Cfg:       AgentConfig{MaxSteps: 5},
	}
	ans, err := loop.Run(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ans.gaveUp() {
		t.Fatalf("ungrounded answer must become give_up, got %+v", ans)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'TestAgentLoop_AnswerWithoutChunks_GivesUp|TestGroundingCheck_|TestAgentLoop_RetrieveThenAnswer_HappyPath|TestAgentLoop_UngroundedAnswer_BecomesGiveUp'`
  Expected: FAIL (`doAnswer not implemented`, undefined `isGrounded`).

- [ ] **Step 3: Replace the `doAnswer` stub + add `isGrounded`** — in `internal/memory/agent_loop.go`

```go
// doAnswer builds context from accumulated chunks, generates, and runs the
// grounding check. An answer with no chunks, or one whose citations don't all
// reference accumulated chunks, becomes an honest give_up rather than the
// generator's (possibly hallucinated) text.
func (a *AgentLoop) doAnswer(ctx context.Context, state AgentState, q Query) (Answer, error) {
	if len(state.AccumulatedChunks) == 0 {
		return a.doGiveUp("no chunks"), nil
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
		return a.doGiveUp("answer ungrounded"), nil
	}
	return ans, nil
}

// isGrounded is the cheap grounding check: the answer must carry at least one
// [source:id] citation, and EVERY citation must reference an id in the supplied
// chunk-source set. (A fuller grounding-LLM call is a future rung.)
func isGrounded(text string, sourceIDs []string) bool {
	matches := citationRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return false
	}
	allowed := make(map[string]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		allowed[id] = true
	}
	for _, m := range matches {
		if !allowed[m[1]] {
			return false
		}
	}
	return true
}
```

> `citationRe` already exists in [generator.go:74](../../../internal/memory/generator.go#L74) (same package) — reuse it, do not redeclare.

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'TestAgentLoop_|TestGroundingCheck_'`
  Expected: PASS (all loop tests).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/agent_loop.go internal/memory/agent_loop_test.go
git commit -m "feat(memory): add doAnswer with grounding check that gives up when ungrounded"
```

---

## Task 7: Config — `AGENT_LOOP_ENABLED` / `AGENT_MAX_STEPS`

**Files:**
- Modify: `internal/memory/config.go`
- Test: `internal/memory/config_test.go`

- [ ] **Step 1: Write the failing test** — append to `internal/memory/config_test.go` (follow the file's existing env-set/Setenv pattern)

```go
func TestLoadConfig_AgentDefaults(t *testing.T) {
	setRequiredEnv(t) // existing helper: sets DATABASE_URL, EMBEDDING_*, BRAIN_*, EMBEDDING_DIM
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Agent.Enabled {
		t.Fatal("AGENT_LOOP_ENABLED must default to false")
	}
	if cfg.Agent.MaxSteps != 5 {
		t.Fatalf("AGENT_MAX_STEPS default = %d, want 5", cfg.Agent.MaxSteps)
	}
}

func TestLoadConfig_AgentOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AGENT_LOOP_ENABLED", "true")
	t.Setenv("AGENT_MAX_STEPS", "8")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Agent.Enabled || cfg.Agent.MaxSteps != 8 {
		t.Fatalf("agent overrides not applied: %+v", cfg.Agent)
	}
}
```

> If `setRequiredEnv` doesn't exist under that name, mirror whatever the existing `config_test.go` cases use to satisfy the required-env preconditions (it sets DATABASE_URL/EMBEDDING_*/BRAIN_*/EMBEDDING_DIM).

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'TestLoadConfig_Agent'`
  Expected: FAIL (undefined: `cfg.Agent`).

- [ ] **Step 3: Write minimal implementation** — in `internal/memory/config.go`

  Add the struct:

```go
// AgentLoopConfig holds the rung-1 agent-loop knobs. Default-off: when Enabled
// is false, Orchestrator.Answer runs the legacy pipeline unchanged.
type AgentLoopConfig struct {
	Enabled  bool // AGENT_LOOP_ENABLED (default false)
	MaxSteps int  // AGENT_MAX_STEPS (default 5, hard cap on loop iterations)
}
```

  Add the field to `MemoryConfig`:

```go
	// Agentic RAG (rung 1) knobs.
	Agent AgentLoopConfig
```

  Load it near the end of `LoadConfig` (before `return cfg, nil`):

```go
	cfg.Agent = AgentLoopConfig{
		Enabled:  boolEnv("AGENT_LOOP_ENABLED", false),
		MaxSteps: intEnv("AGENT_MAX_STEPS", 5),
	}
```

> `intEnv` returns the fallback for non-positive values, so `AGENT_MAX_STEPS=0` safely yields 5 — the budget can never be zero.

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'TestLoadConfig_Agent'`
  Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go
git commit -m "feat(memory): add AGENT_LOOP_ENABLED and AGENT_MAX_STEPS config"
```

---

## Task 8: Wire the flag into `Orchestrator.Answer` + `NewModule`

**Files:**
- Modify: `internal/memory/orchestrator.go`
- Modify: `internal/memory/module.go`
- Test: `internal/memory/orchestrator_test.go`

- [ ] **Step 1: Write the failing test** — append to `internal/memory/orchestrator_test.go`

```go
func TestOrchestrator_Answer_DelegatesToAgentLoopWhenSet(t *testing.T) {
	gen := &stubGenerator{text: "x [source:a#0]", cites: []string{"a#0"}}
	o := &Orchestrator{
		Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "fact")}},
		Generator: gen,
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Fusion: PassthroughFusion{},
		AgentLoop: &AgentLoop{
			Planner: &stubPlanner{actions: []AgentAction{
				{Type: ActionRetrieve, Query: "a"}, {Type: ActionAnswer},
			}},
			Retriever: stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "fact")}},
			Generator: gen,
			Builder:   BudgetContextBuilder{MaxRunes: 6000},
			Cfg:       AgentConfig{MaxSteps: 5},
		},
	}
	ans, err := o.Answer(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(ans.Citations) != 1 {
		t.Fatalf("expected agent-loop answer with citation, got %+v", ans)
	}
}

func TestOrchestrator_Answer_LegacyPathWhenAgentLoopNil(t *testing.T) {
	gen := &stubGenerator{text: "legacy [source:a#0]", cites: []string{"a#0"}}
	o := &Orchestrator{
		Retriever:      stubRetriever{chunks: []RetrievedChunk{chunk("a#0", "fact")}},
		Generator:      gen,
		ContextBuilder: BudgetContextBuilder{MaxRunes: 6000},
		Fusion:         PassthroughFusion{},
		// AgentLoop nil → legacy path
	}
	if _, err := o.Answer(context.Background(), Query{Text: "q"}); err != nil {
		t.Fatalf("legacy Answer: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("legacy path should call generator once, calls=%d", gen.calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/ -run 'TestOrchestrator_Answer_'`
  Expected: FAIL (unknown field `AgentLoop` in Orchestrator).

- [ ] **Step 3: Implement** — in `internal/memory/orchestrator.go`

  Add the field to the `Orchestrator` struct (after `Generator`):

```go
	// AgentLoop, when non-nil, is the rung-1 agentic path. NewModule sets it only
	// when AGENT_LOOP_ENABLED=true; when nil, Answer runs the legacy pipeline.
	AgentLoop *AgentLoop
```

  At the top of `Answer`, before the legacy body:

```go
func (o *Orchestrator) Answer(ctx context.Context, q Query) (Answer, error) {
	if o.AgentLoop != nil {
		return o.AgentLoop.Run(ctx, q)
	}
	// ... existing legacy body unchanged ...
```

  In `internal/memory/module.go` `NewModule`, after building `generator`, build the loop only when enabled and attach it:

```go
	orch := &Orchestrator{
		// ... existing fields unchanged ...
	}
	if cfg.Agent.Enabled {
		orch.AgentLoop = &AgentLoop{
			Retriever: orch.Retriever,
			Builder:   orch.ContextBuilder,
			Generator: orch.Generator,
			Planner:   NewBrainPlanner(generator),
			Cfg:       AgentConfig{MaxSteps: cfg.Agent.MaxSteps},
			Depth:     orch.Depth,
			FinalK:    orch.FinalK,
		}
	}
	return orch, conn, nil
```

> Refactor `NewModule`'s `return &Orchestrator{...}` into the named `orch` variable shown above so the loop can reference the wired dependencies. Behavior with the flag off is identical to today.

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/ -run 'TestOrchestrator_Answer_'`
  Expected: PASS.

- [ ] **Step 5: Full package test** — Run: `go test ./internal/memory/...`
  Expected: PASS (no regression in existing orchestrator/recall tests).

- [ ] **Step 6: Commit**

```bash
git add internal/memory/orchestrator.go internal/memory/module.go internal/memory/orchestrator_test.go
git commit -m "feat(memory): delegate Answer to agent loop behind AGENT_LOOP_ENABLED flag"
```

---

## Task 9: `-agent` eval mode (give-up + grounding gate)

**Files:**
- Create: `internal/memory/eval/agent.go`
- Modify: `internal/cli/memory/eval.go`
- Modify: `Makefile`
- Test: `internal/memory/eval/agent_test.go`

This is the user-approved resolution to the eval-gate problem: a generation-path metric the retrieval-only harness can't capture. Retrieval recall@5 stays the no-regression guard; this adds give-up correctness + grounding pass rate.

- [ ] **Step 1: Write the failing test** — `internal/memory/eval/agent_test.go`

```go
package eval

import "testing"

func TestScoreAgentResult_NegativeExpectsGiveUp(t *testing.T) {
	// negative question (Expected empty): give_up is correct.
	r := AgentResult{Question: Question{QuestionType: QuestionNegative}, GaveUp: true}
	if !scoreGiveUpCorrect(r) {
		t.Fatal("negative question + give_up should score correct")
	}
	r2 := AgentResult{Question: Question{QuestionType: QuestionNegative}, GaveUp: false}
	if scoreGiveUpCorrect(r2) {
		t.Fatal("negative question + answer should score incorrect")
	}
}

func TestScoreAgentResult_KnownExpectsGroundedAnswer(t *testing.T) {
	r := AgentResult{
		Question: Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"d"}},
		GaveUp:   false,
		Citations: []string{"d#0"},
	}
	if !scoreGiveUpCorrect(r) {
		t.Fatal("known question + grounded answer should score correct")
	}
	r2 := AgentResult{Question: Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"d"}}, GaveUp: true}
	if scoreGiveUpCorrect(r2) {
		t.Fatal("known question + give_up should score incorrect")
	}
}

func TestSummarizeAgent_Aggregates(t *testing.T) {
	results := []AgentResult{
		{Question: Question{QuestionType: QuestionNegative}, GaveUp: true},
		{Question: Question{QuestionType: QuestionParaphrase, ExpectedDocIDs: []string{"d"}}, GaveUp: false, Citations: []string{"d#0"}},
	}
	s := SummarizeAgent(results)
	if s.GiveUpCorrect != 2 || s.Total != 2 {
		t.Fatalf("expected 2/2 correct, got %d/%d", s.GiveUpCorrect, s.Total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — Run: `go test ./internal/memory/eval/ -run 'AgentResult|SummarizeAgent'`
  Expected: FAIL (undefined: `AgentResult`).

- [ ] **Step 3: Write minimal implementation** — `internal/memory/eval/agent.go`

```go
package eval

import (
	"context"
	"fmt"

	"github.com/luannn010/ptolemy/internal/memory"
)

// AgentResult is one question run through the agent loop (not just retrieval).
type AgentResult struct {
	Question  Question
	GaveUp    bool
	Citations []string
}

// AgentSummary aggregates give-up/grounding correctness over a seed.
type AgentSummary struct {
	Total         int
	GiveUpCorrect int // questions where give_up vs answer matched expectation
	Grounded      int // non-give_up answers carrying >=1 citation
	Answered      int // non-give_up count (denominator for grounding)
}

// answerer is the loop surface the agent eval needs. *memory.Orchestrator
// satisfies it (Answer delegates to the loop when the flag is on).
type answerer interface {
	Answer(ctx context.Context, q memory.Query) (memory.Answer, error)
}

// scoreGiveUpCorrect: negative questions (Expected empty) are correct iff the
// loop gave up; all other (answerable) questions are correct iff it answered.
func scoreGiveUpCorrect(r AgentResult) bool {
	answerable := len(r.Question.ExpectedDocIDs) > 0
	if answerable {
		return !r.GaveUp
	}
	return r.GaveUp
}

// RunAgentEval runs every seed question through the agent loop and scores
// give-up correctness + grounding. Unlike RunRetrieval this DOES call the LLM,
// so it is slow (one+ BRAIN call per step per question).
func RunAgentEval(ctx context.Context, a answerer, s Seed) ([]AgentResult, error) {
	results := make([]AgentResult, 0, len(s.Questions))
	for _, q := range s.Questions {
		ans, err := a.Answer(ctx, memory.Query{Text: q.Text, K: s.K})
		if err != nil {
			return nil, fmt.Errorf("agent answer %s: %w", q.ID, err)
		}
		results = append(results, AgentResult{Question: q, GaveUp: ans.GaveUp, Citations: ans.Citations})
	}
	return results, nil
}

func SummarizeAgent(results []AgentResult) AgentSummary {
	out := AgentSummary{Total: len(results)}
	for _, r := range results {
		if scoreGiveUpCorrect(r) {
			out.GiveUpCorrect++
		}
		if !r.GaveUp {
			out.Answered++
			if len(r.Citations) > 0 {
				out.Grounded++
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes** — Run: `go test ./internal/memory/eval/ -run 'AgentResult|SummarizeAgent|ScoreAgent'`
  Expected: PASS.

- [ ] **Step 5: Wire the `-agent` flag** — in `internal/cli/memory/eval.go`

  Add a flag and a branch (after the existing flag declarations and ingest):

```go
	agentMode := fs.Bool("agent", false, "run questions through the agent loop and score give-up/grounding (requires AGENT_LOOP_ENABLED=true)")
```

  After ingest, before the retrieval block:

```go
	if *agentMode {
		results, err := eval.RunAgentEval(ctx, orch, seed)
		if err != nil {
			return fmt.Errorf("agent eval: %w", err)
		}
		for _, r := range results {
			verb := "ANSWER"
			if r.GaveUp {
				verb = "GIVEUP"
			}
			fmt.Fprintf(stdout, "[%s] %s  cites=%v\n", verb, r.Question.ID, r.Citations)
		}
		s := eval.SummarizeAgent(results)
		fmt.Fprintf(stdout, "\ngive_up_correct = %d/%d   grounded = %d/%d answered\n",
			s.GiveUpCorrect, s.Total, s.Grounded, s.Answered)
		return nil
	}
```

- [ ] **Step 6: Add Makefile target** — in `Makefile`, after `eval-memory`:

```makefile
eval-memory-agent: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	RAG_CHUNK_SIZE_TOKENS=$(EVAL_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	AGENT_LOOP_ENABLED=true \
	  $(BIN_DIR)/ptolemy memory eval -seed $(EVAL_SEED) -agent
```

- [ ] **Step 7: Verify build + eval-pkg tests** — Run: `go build ./... && go test ./internal/memory/eval/ ./internal/cli/memory/`
  Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/memory/eval/agent.go internal/memory/eval/agent_test.go internal/cli/memory/eval.go Makefile
git commit -m "feat(memory): add -agent eval mode scoring give-up and grounding"
```

---

## Task 10: Run the eval gate (§3.7) + integration tests + docs

**Files:**
- Modify: `docs/Architecture.md`
- (Optional) Create: `internal/memory/agent_loop_integration_test.go` (real BRAIN, build-tagged)

- [ ] **Step 1: Baseline — retrieval gate, flag off** — Run: `make eval-memory`
  Record per-type + overall recall@5 (expected: 1.000/30, the current baseline).

- [ ] **Step 2: Retrieval no-regression, flag on** — Run: `AGENT_LOOP_ENABLED=true make eval-memory`
  Note: `eval-memory` uses `RunRetrieval` (retriever directly), so the flag does not change this number — it confirms wiring didn't disturb retrieval. Expected: 1.000/30 (no regression, the §3.7 ≤1pp guard).

- [ ] **Step 3: Agent gate — give-up + grounding** — Run: `make eval-memory-agent`
  Record `give_up_correct = X/30` and `grounded = Y/Z answered`. The meaningful new signal: the 2 negative questions should produce GIVEUP; answerable questions should produce grounded answers. Capture this output for the PR.

- [ ] **Step 4 (optional but recommended): real-BRAIN integration tests** — `internal/memory/agent_loop_integration_test.go` with `//go:build integration`, mirroring the eval seed:
  - `TestAgentLoop_EndToEnd_KnownQuestion` — a paraphrase seed question → `!GaveUp`, ≥1 citation.
  - `TestAgentLoop_EndToEnd_UnanswerableQuestion` — `n01-no-grpc` → `GaveUp == true`.

  Run: `set -a; . ./.env; set +a; go test -tags=integration -run TestAgentLoop_EndToEnd ./internal/memory/ -v`

- [ ] **Step 5: Coverage gate (80%)** — Run: `go test -cover ./internal/...`
  Confirm ≥80%. If the agent-loop files drag it down, the unit tests in Tasks 5–6 should already cover the branches; add table cases for any uncovered error path.

- [ ] **Step 6: Architecture note** — append one paragraph to `docs/Architecture.md` describing the rung-1 agent loop (planner + grammar + budget + grounding), the flag default-off, and the `-agent` eval mode. Per the package DoD.

- [ ] **Step 7: Full suite** — Run: `make test`
  Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add docs/Architecture.md internal/memory/agent_loop_integration_test.go
git commit -m "docs(memory): document rung-1 agent loop; add end-to-end eval gate"
```

---

## PR description checklist (acceptance §5)

- [ ] Discovery answers (§0 Q1–Q4) — copy the Discovery Summary above.
- [ ] Eval delta: retrieval recall@5 flag-off vs flag-on (no >1pp regression) **and** the new `-agent` give-up/grounding numbers. If give-up correctness or grounding is weak, document it as a null/partial result and keep `AGENT_LOOP_ENABLED` **off**.
- [ ] Confirm flag defaults off; legacy path unchanged.
- [ ] Coverage ≥80% on `./internal/...`.
- [ ] No new infrastructure.
- [ ] Out-of-scope rungs (query rewriting, multi-hop `judge_sufficient`, routing, tool-use) listed as follow-ups.
- [ ] Run `superpowers:requesting-code-review` before opening the PR (per CLAUDE.md / AGENTS.md). Use the GitHub plugin tooling, not `gh`.

## Notes / risks

- **Latency:** each loop step is a BRAIN call (warm ~0.2s, cold ~5.6s). `make eval-memory-agent` runs 30 questions × up-to-5 steps — budget for minutes, not seconds, on a cold model.
- **Grammar always-present fields:** the action grammar requires `query` and `reason` keys always (empty when unused). Simpler grammar + honest drift guard; validators enforce semantics.
- **`citationRe` reuse:** `isGrounded` reuses the package-level `citationRe` from generator.go — do not redeclare.
- **No eval-set change:** `seed.json` is untouched (prompt requirement). The new gate adds a *mode*, not new questions.
