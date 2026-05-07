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
			t.Fatalf("expected executor tool schema to expose %s", field)
		}
	}
}
