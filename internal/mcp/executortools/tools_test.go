package executortools

import "testing"

func TestExecutorToolsRegistered(t *testing.T) {
	tools := Tools()

	if len(tools) != 1 {
		t.Fatalf("expected 1 executor tool, got %d", len(tools))
	}

	if tools[0].Name != "ptolemy.execute" {
		t.Fatalf("expected ptolemy.execute, got %s", tools[0].Name)
	}

	properties, ok := tools[0].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected executor tool properties schema, got %#v", tools[0].InputSchema["properties"])
	}

	for _, field := range []string{"title", "purpose", "reasoning_step", "target"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected executor tool property %q to be present", field)
		}
	}

	required, ok := tools[0].InputSchema["required"].([]string)
	if !ok {
		t.Fatalf("expected required list as []string, got %#v", tools[0].InputSchema["required"])
	}

	expectedRequired := map[string]bool{
		"session_id": true,
		"command":    true,
	}
	if len(required) != len(expectedRequired) {
		t.Fatalf("expected only legacy required fields %v, got %v", expectedRequired, required)
	}

	for _, field := range required {
		if !expectedRequired[field] {
			t.Fatalf("unexpected required field %q", field)
		}
		delete(expectedRequired, field)
	}

	if len(expectedRequired) != 0 {
		t.Fatalf("missing required fields: %v", expectedRequired)
	}
}
