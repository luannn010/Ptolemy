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

// doAnswer is implemented in Task 6 (grounding check). Temporary stub so the
// loop compiles; the ActionAnswer path is not exercised by Task 5 tests.
func (a *AgentLoop) doAnswer(ctx context.Context, state AgentState, q Query) (Answer, error) {
	return Answer{}, fmt.Errorf("doAnswer not implemented")
}
