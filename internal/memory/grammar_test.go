package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestGrammarFile_ValidGBNF(t *testing.T) {
	b, err := os.ReadFile("grammar/atom.gbnf")
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, `\"`, `"`)
	if s == "" {
		t.Fatal("grammar is empty")
	}
	for _, required := range []string{"root ::=", "\"atoms\"", "perspective ::="} {
		if !strings.Contains(s, required) {
			t.Fatalf("grammar missing %q", required)
		}
	}
}

func TestGrammarMatchesAtomStruct(t *testing.T) {
	b, err := os.ReadFile("grammar/atom.gbnf")
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	s := string(b)
	s = strings.ReplaceAll(s, `\"`, `"`)
	rt := reflect.TypeOf(Atom{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" {
			continue
		}
		if !strings.Contains(s, `"`+tag+`"`) {
			t.Fatalf("grammar missing field %q from Atom json tag", tag)
		}
	}
}

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

func TestExtractor_SendsGrammarInRequest(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"atoms":[]}`}},
			},
		})
	}))
	defer srv.Close()

	e := NewExtractor(NewOpenAIGenerator(srv.URL, "test-model", ""))
	_, err := e.Extract(context.Background(), Exchange{UserText: "u", AssistantText: "a"})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	grammar, _ := got["grammar"].(string)
	if strings.TrimSpace(grammar) == "" {
		t.Fatal("expected grammar field in request body")
	}
}
