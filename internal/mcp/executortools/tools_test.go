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
}
