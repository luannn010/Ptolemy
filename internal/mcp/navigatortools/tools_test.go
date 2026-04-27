package navigatortools

import "testing"

func TestNavigatorToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy.index_workspace":     false,
		"ptolemy.read_context":        false,
		"ptolemy.start_task_session":  false,
		"ptolemy.append_session_note": false,
	}

	for _, tool := range tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("expected tool %s to be registered", name)
		}
	}
}
