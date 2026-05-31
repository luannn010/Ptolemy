package memory

import "testing"

func TestActionValidator_RejectsUnknownType(t *testing.T) {
	if err := validateAction(AgentAction{Type: "frobnicate"}); err == nil {
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

func TestActionValidator_AcceptsValidActions(t *testing.T) {
	cases := []AgentAction{
		{Type: ActionAnswer},
		{Type: ActionRetrieve, Query: "rrf constant"},
		{Type: ActionGiveUp, Reason: "not in knowledge base"},
	}
	for _, a := range cases {
		if err := validateAction(a); err != nil {
			t.Fatalf("action %+v should validate: %v", a, err)
		}
	}
}
