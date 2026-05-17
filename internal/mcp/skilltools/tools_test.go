package skilltools

import (
	"encoding/json"
	"testing"
)

func TestSkillToolsRegistered(t *testing.T) {
	tools := Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 skill tool, got %d", len(tools))
	}
	if tools[0].Name != "ptolemy_list_skills" {
		t.Fatalf("expected ptolemy_list_skills, got %s", tools[0].Name)
	}
}

func TestHandleListSkills(t *testing.T) {
	result, handled, err := Handle("ptolemy_list_skills", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected tool to be handled")
	}

	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected text content in result, got %#v", result["content"])
	}

	text, ok := content[0]["text"].(string)
	if !ok {
		t.Fatalf("expected content text string, got %#v", content[0]["text"])
	}

	var payload struct {
		Count  int `json:"count"`
		Skills []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Category    string `json:"category"`
		} `json:"skills"`
	}

	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("expected valid JSON payload, got error: %v", err)
	}

	if payload.Count == 0 || len(payload.Skills) == 0 {
		t.Fatalf("expected non-empty skills payload, got count=%d len=%d", payload.Count, len(payload.Skills))
	}
}
