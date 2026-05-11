package sessiontools

import "testing"

func TestSessionToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy_create_session": false,
		"ptolemy_list_sessions":  false,
		"ptolemy_get_session":    false,
		"ptolemy_close_session":  false,
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
