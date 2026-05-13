package terminal

import (
	"context"
	"errors"
	"testing"
)

func TestTmuxRunnerFallsBackWhenTmuxIsUnavailable(t *testing.T) {
	originalLookPath := lookPath
	lookPath = func(file string) (string, error) {
		if file == "tmux" {
			return "", errors.New("not found")
		}
		return originalLookPath(file)
	}
	defer func() { lookPath = originalLookPath }()

	runner := NewTmuxRunner()

	result := runner.Run(context.Background(), "session-id", "echo fallback-ready", "", 5)

	if result.ExitCode != 0 {
		t.Fatalf("expected fallback command to succeed, got exit code %d: %s", result.ExitCode, result.ErrorOutput)
	}
	if result.Output != "fallback-ready\n" {
		t.Fatalf("expected fallback output, got %q", result.Output)
	}
}

func TestTmuxBootstrapIsNoopWhenTmuxIsUnavailable(t *testing.T) {
	originalLookPath := lookPath
	lookPath = func(file string) (string, error) {
		if file == "tmux" {
			return "", errors.New("not found")
		}
		return originalLookPath(file)
	}
	defer func() { lookPath = originalLookPath }()

	runner := NewTmuxRunner()

	if err := runner.BootstrapSession(context.Background(), "session-id", ""); err != nil {
		t.Fatalf("expected bootstrap to succeed without tmux, got %v", err)
	}
}

func TestExtractMarkedExitCodeHandlesMarkerJoinedToOutput(t *testing.T) {
	output := "hello__PTOLEMY_EXIT_test__:0\n__PTOLEMY_END_test__\n"

	code := extractMarkedExitCode(output, "__PTOLEMY_EXIT_test__")

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestExtractMarkedOutputRemovesJoinedExitMarker(t *testing.T) {
	output := "__PTOLEMY_START_test__\nhello__PTOLEMY_EXIT_test__:0\n__PTOLEMY_END_test__\n"

	cleaned := extractMarkedOutput(output, "__PTOLEMY_START_test__", "__PTOLEMY_EXIT_test__")

	if cleaned != "hello\n" {
		t.Fatalf("expected clean output, got %q", cleaned)
	}
}
