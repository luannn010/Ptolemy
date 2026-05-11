package gittools

import "testing"

func TestGitToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy_git_status":        false,
		"ptolemy_git_diff":          false,
		"ptolemy_git_log":           false,
		"ptolemy_git_checkout":      false,
		"ptolemy_git_create_branch": false,
		"ptolemy_git_commit":        false,
		"ptolemy_git_push":          false,
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
