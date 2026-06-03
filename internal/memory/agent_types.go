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
// Across completed non-terminal iterations StepCount == len(Steps); on the
// terminating iteration Steps has one more entry than StepCount (the loop
// appends the terminal action, then returns before incrementing). StepCount is
// kept as a field (not derived) because the loop reads it directly as the
// budget guard.
type AgentState struct {
	Query             string
	AccumulatedChunks []RetrievedChunk
	Steps             []AgentAction // history of actions taken
	StepCount         int
	Budget            int // AGENT_MAX_STEPS
}

// BudgetRemaining is how many steps the loop may still take before the budget
// forces a give_up.
func (s AgentState) BudgetRemaining() int { return s.Budget - s.StepCount }

// Planner returns the next AgentAction given the current state.
type Planner interface {
	NextAction(ctx context.Context, state AgentState) (AgentAction, error)
}

// AgentConfig holds the loop's runtime knobs. MaxSteps is sourced from
// AGENT_MAX_STEPS (default 5 is applied in config loading); a non-positive
// value here means the loop falls back to its own default.
type AgentConfig struct {
	MaxSteps int
}
