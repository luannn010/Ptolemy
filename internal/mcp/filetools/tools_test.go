package filetools

import "testing"

func TestFileToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy_read_file":       false,
		"ptolemy_write_file":      false,
		"ptolemy_list_directory":  false,
		"ptolemy_search_codebase": false,
		"ptolemy_apply_patch":     false,
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
