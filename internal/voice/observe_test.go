package voice

import (
	"strings"
	"testing"
)

func TestFormatObservationContainsFields(t *testing.T) {
	obs := Observation{
		Heard:    `hey ptolemy run go test`,
		Caught:   "run_shell",
		Request:  `{"command":"go test ./..."}`,
		Tools:    []string{"open_session", "execute"},
		TimingMS: 412,
	}
	got := FormatObservation(obs, true)

	for _, want := range []string{
		"hey ptolemy run go test",
		"run_shell",
		`{"command":"go test ./..."}`,
		"open_session, execute",
		"412ms",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted observation missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestFormatObservationUsesDimWhenColorEnabled(t *testing.T) {
	obs := Observation{Heard: "x", Caught: "run_shell"}
	got := FormatObservation(obs, true)
	if !strings.Contains(got, "\x1b[2m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("expected ANSI dim codes when color enabled, got: %q", got)
	}
}

func TestFormatObservationNoColorWhenDisabled(t *testing.T) {
	obs := Observation{Heard: "x", Caught: "run_shell"}
	got := FormatObservation(obs, false)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected no ANSI codes when color disabled, got: %q", got)
	}
}

func TestFormatObservationOmitsEmptyOptionalLines(t *testing.T) {
	// Only "heard" present: should not render request/tools/timing lines.
	obs := Observation{Heard: "just listening"}
	got := FormatObservation(obs, false)
	if strings.Contains(got, "request:") {
		t.Fatalf("did not expect a request line when Request is empty:\n%s", got)
	}
	if strings.Contains(got, "tools:") {
		t.Fatalf("did not expect a tools line when Tools is empty:\n%s", got)
	}
	if strings.Contains(got, "timing:") {
		t.Fatalf("did not expect a timing line when TimingMS is zero:\n%s", got)
	}
	if !strings.Contains(got, "heard:") {
		t.Fatalf("expected a heard line:\n%s", got)
	}
}

func TestTrimPayloadShortensLongJSON(t *testing.T) {
	long := `{"session_id":"` + strings.Repeat("a", 300) + `","command":"go test"}`
	got := trimPayload(long, 80)
	if len(got) > 84 { // 80 + ellipsis allowance
		t.Fatalf("expected trimmed payload <= ~84 chars, got %d: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix on trimmed payload, got: %q", got)
	}
}
