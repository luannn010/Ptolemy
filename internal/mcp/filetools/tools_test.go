package filetools

import (
	"testing"
)

func TestFileToolsRegistered(t *testing.T) {
	tools := Tools()

	expected := map[string]bool{
		"ptolemy.read_file":       false,
		"ptolemy.write_file":      false,
		"ptolemy.list_directory":  false,
		"ptolemy.search_codebase": false,
		"ptolemy.apply_patch":     false,
		"ptolemy.replace_block":   false,
		"ptolemy.insert_after":    false,
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

func TestTargetedEditToolsRouteToWorkerEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		wantPath string
	}{
		{
			name:     "replace block",
			tool:     "ptolemy.replace_block",
			wantPath: "/file/replace_block",
		},
		{
			name:     "insert after",
			tool:     "ptolemy.insert_after",
			wantPath: "/file/insert_after",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, handled := workerPath(tt.tool)
			if gotPath != tt.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if !handled {
				t.Fatal("expected tool to be handled")
			}
		})
	}
}
