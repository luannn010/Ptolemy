package memory

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

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

func (a *AgentLoop) doGiveUp(reason string) Answer {
	log.Info().Str("stage", "agent_give_up").Str("reason", reason).Msg("agent loop: give up")
	return Answer{Text: "I don't know based on what I found: " + reason, GaveUp: true}
}

// doAnswer builds context from accumulated chunks, generates, and runs the
// grounding check. An answer with no chunks, or one whose citations don't all
// reference accumulated chunks, becomes an honest give_up rather than the
// generator's (possibly hallucinated) text.
func (a *AgentLoop) doAnswer(ctx context.Context, state AgentState, q Query, tr *RecallTrace) (Answer, error) {
	if len(state.AccumulatedChunks) == 0 {
		traceGiveUp(tr, "no chunks")
		return attachTrace(tr, a.doGiveUp("no chunks")), nil
	}
	// finalK precedence: an explicit per-query q.K wins; otherwise fall back to the
	// loop's a.FinalK. A resulting finalK <= 0 means "no count cap" here — the
	// ContextBuilder's rune budget is then the only limiter.
	finalK := q.K
	if finalK <= 0 {
		finalK = a.FinalK
	}
	// finalK caps the chunk COUNT pre-build; the Builder may still drop more under
	// its rune budget, so pc.SourceIDs (below) — not finalK — is the authority on
	// what actually reached the prompt, and grounding is checked against that.
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
