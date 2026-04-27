package terminal

import "testing"

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
