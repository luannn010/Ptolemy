package gittools

import "testing"

func TestGitToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy.git_status":        false,
		"ptolemy.git_diff":          false,
		"ptolemy.git_log":           false,
		"ptolemy.git_checkout":      false,
		"ptolemy.git_create_branch": false,
		"ptolemy.git_commit":        false,
		"ptolemy.git_push":          false,
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
