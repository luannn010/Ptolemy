package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeErrChat is a ChatClient whose Complete always fails, to exercise the
// planner's transport-error wrap.
type fakeErrChat struct{ err error }

func (f fakeErrChat) Complete(_ context.Context, _, _ string, _ CompleteOptions) (string, error) {
	return "", f.err
}

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
	if err := validateAction(AgentAction{Type: ActionGiveUp, Reason: "   "}); err == nil {
		t.Fatal("expected empty give_up reason to be rejected")
	}
}

func TestActionValidator_AcceptsValidActions(t *testing.T) {
	cases := []AgentAction{
		{Type: ActionAnswer},
		{Type: ActionRetrieve, Query: "rrf constant"},
		{Type: ActionRetrieve, Query: "  rrf  "},
		{Type: ActionGiveUp, Reason: "not in knowledge base"},
	}
	for _, a := range cases {
		if err := validateAction(a); err != nil {
			t.Fatalf("action %+v should validate: %v", a, err)
		}
	}
}

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

func TestBrainPlanner_ParseErrorOnGarbage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "not json at all"}},
			},
		})
	}))
	defer srv.Close()
	p := NewBrainPlanner(NewOpenAIGenerator(srv.URL, "test-model", ""))
	if _, err := p.NextAction(context.Background(), AgentState{Query: "q", Budget: 5}); err == nil {
		t.Fatal("expected parse error on non-JSON planner output")
	}
}

func TestBrainPlanner_WrapsTransportError(t *testing.T) {
	p := NewBrainPlanner(fakeErrChat{err: errors.New("boom")})
	_, err := p.NextAction(context.Background(), AgentState{Query: "q", Budget: 5})
	if err == nil {
		t.Fatal("expected transport error to propagate")
	}
	if !strings.Contains(err.Error(), "planner llm") {
		t.Fatalf("expected error wrapped with %q, got %v", "planner llm", err)
	}
}

func TestBrainPlanner_AnswerActionPassesThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"type":"answer","query":"","reason":""}`}},
			},
		})
	}))
	defer srv.Close()
	p := NewBrainPlanner(NewOpenAIGenerator(srv.URL, "test-model", ""))
	act, err := p.NextAction(context.Background(), AgentState{Query: "q", Budget: 5})
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}
	if act.Type != ActionAnswer {
		t.Fatalf("expected answer action, got %+v", act)
	}
}
