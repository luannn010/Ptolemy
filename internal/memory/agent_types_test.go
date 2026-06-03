package memory

import (
	"encoding/json"
	"testing"
)

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
	if got := s.BudgetRemaining(); got != 3 {
		t.Fatalf("BudgetRemaining = %d, want 3", got)
	}
}

func TestAgentAction_JSONRoundTrip(t *testing.T) {
	in := AgentAction{Type: ActionRetrieve, Query: "rrf constant", Reason: ""}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Tags must serialize to the lowercase keys the planner grammar emits.
	if got := string(b); got != `{"type":"retrieve","query":"rrf constant","reason":""}` {
		t.Fatalf("unexpected JSON: %s", got)
	}
	var out AgentAction
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip mismatch: %+v != %+v", out, in)
	}
}
