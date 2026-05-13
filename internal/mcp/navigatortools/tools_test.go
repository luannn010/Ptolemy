package navigatortools

import "testing"

func TestNavigatorToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy_index_workspace":     false,
		"ptolemy_read_context":        false,
		"ptolemy_start_task_session":  false,
		"ptolemy_append_session_note": false,
		"ptolemy_kb_build":            false,
		"ptolemy_kb_read":             false,
		"ptolemy_kb_update":           false,
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
