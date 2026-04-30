package tasks

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTaskExecutionContractIncludesAssetsAndConstraints(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "task-scripts"))
	mustMkdir(t, filepath.Join(root, "snippets"))
	mustWriteFile(t, filepath.Join(root, "task-scripts", "step.md"), "# step\n")
	mustWriteFile(t, filepath.Join(root, "snippets", "code.txt"), "snippet body\n")

	task := Task{
		ID:                  "task-1",
		Branch:              "ptolemy/task-1",
		AllowedFiles:        []string{"internal/tasks/example.go"},
		Validation:          []string{"go test ./internal/tasks"},
		Scripts:             []string{"task-scripts/step.md"},
		Snippets:            []string{"snippets/code.txt"},
		MaxSteps:            8,
		RequiresApproval:    false,
		StopOnError:         true,
		Sections: map[string]string{
			"Goal":                 "Goal text",
			"Scope":                "Scope text",
			"Constraints":          "Constraint text",
			"Inputs":               "- `task-scripts/step.md`\n- `snippets/code.txt`",
			"Execution Steps":      "Do the work",
			"Acceptance Checks":    "Run tests",
			"Failure / Escalation": "Stop on failure",
			"Done When":            "Validation passes",
		},
		PackContext: &TaskPackContext{
			Root:              root,
			PackID:            "sample-pack",
			TaskScriptsDir:    "task-scripts",
			SnippetsDir:       "snippets",
			AgentMode:         AgentModeBoundedMarkdownContract,
			GlobalConstraints: []string{"do not edit files outside allowed_files"},
		},
	}

	contract, err := BuildTaskExecutionContract(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(contract, "Ptolemy Task Execution Contract") {
		t.Fatalf("expected contract header, got %s", contract)
	}
	if !strings.Contains(contract, "task-scripts/step.md") || !strings.Contains(contract, "snippet body") {
		t.Fatalf("expected asset content, got %s", contract)
	}
	if !strings.Contains(contract, "do not edit files outside allowed_files") {
		t.Fatalf("expected global constraints, got %s", contract)
	}
}
