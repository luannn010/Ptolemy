package terminal

import (
	"context"
	"testing"
)

func TestRunnerRunSuccess(t *testing.T) {
	runner := NewRunner()

	result := runner.Run(context.Background(), "echo hello", "", 5)

	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	if result.Output != "hello\n" {
		t.Fatalf("expected hello output, got %q", result.Output)
	}
}

func TestRunnerRunFailure(t *testing.T) {
	runner := NewRunner()

	result := runner.Run(context.Background(), "exit 7", "", 5)

	if result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", result.ExitCode)
	}
}

func TestRunnerRunTimeout(t *testing.T) {
	runner := NewRunner()

	result := runner.Run(context.Background(), "sleep 2", "", 1)

	if result.ExitCode != 124 {
		t.Fatalf("expected timeout exit code 124, got %d", result.ExitCode)
	}
}