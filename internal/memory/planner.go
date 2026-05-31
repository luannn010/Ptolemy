package memory

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

var _ Planner = (*BrainPlanner)(nil)

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
	// StepCount is zero-based (steps already taken); +1 renders the 1-indexed
	// current step number for the prompt.
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
