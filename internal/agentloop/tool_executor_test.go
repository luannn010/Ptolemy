package agentloop

import (
	"testing"

	"github.com/luannn010/ptolemy/internal/action"
)

func TestToolExecutorWriteFileRejectsPathOutsideAllowedFiles(t *testing.T) {
	workspace := t.TempDir()
	executor := NewToolExecutor(nil, nil, nil, t.TempDir())

	_, err := executor.writeFile(
		t.Context(),
		Run{ID: "run-1", CurrentStep: 1},
		ActionTask{
			ID:           "task-1",
			Workspace:    workspace,
			AllowedFiles: []string{"README.md"},
		},
		&action.ActionEnvelope{
			Action:  "write_file",
			Path:    "cmd/workerd/main.go",
			Content: "package main",
		},
	)
	if err != ErrPathNotInTaskScope {
		t.Fatalf("expected ErrPathNotInTaskScope, got %v", err)
	}
}

func TestToolExecutorRunCommandDeniesSecretsRead(t *testing.T) {
	executor := NewToolExecutor(nil, nil, nil, t.TempDir())

	result, err := executor.runCommand(
		t.Context(),
		Run{ID: "run-1", CurrentStep: 1},
		ActionTask{
			ID:        "task-1",
			Workspace: t.TempDir(),
		},
		&action.ActionEnvelope{
			Action:  "run_command",
			Command: "cat .env",
		},
	)
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if result.Status != "denied" {
		t.Fatalf("expected denied status, got %q", result.Status)
	}
	if result.ShouldContinue {
		t.Fatal("expected denied command to stop loop continuation")
	}
}
