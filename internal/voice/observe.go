package voice

import (
	"fmt"
	"strings"
)

// Observation holds the diagnostic details for one step of the voice pipeline,
// rendered as a dimmed multi-line block beneath the normal output so you can see
// what was heard, what was sent to the server, which tools ran, and how long it
// took.
type Observation struct {
	Heard    string   // raw recognized phrase ("what you said")
	Caught   string   // the command type the phrase was caught as (e.g. run_shell)
	Request  string   // trimmed JSON payload sent to the server (if any)
	Tools    []string // server tools/endpoints exercised (e.g. open_session, execute)
	TimingMS int64    // total round-trip time for this step, in milliseconds
}

const (
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// FormatObservation renders obs as an indented, dimmed block. Empty optional
// fields are omitted. When color is false, no ANSI codes are emitted (useful for
// non-TTY output or tests).
func FormatObservation(obs Observation, color bool) string {
	var lines []string
	if obs.Heard != "" {
		lines = append(lines, fmt.Sprintf("  heard:   %q", obs.Heard))
	}
	if obs.Caught != "" {
		lines = append(lines, fmt.Sprintf("  caught:  %s", obs.Caught))
	}
	if obs.Request != "" {
		lines = append(lines, fmt.Sprintf("  request: %s", obs.Request))
	}
	if len(obs.Tools) > 0 {
		lines = append(lines, fmt.Sprintf("  tools:   %s", strings.Join(obs.Tools, ", ")))
	}
	if obs.TimingMS > 0 {
		lines = append(lines, fmt.Sprintf("  timing:  %dms", obs.TimingMS))
	}
	if len(lines) == 0 {
		return ""
	}
	block := strings.Join(lines, "\n")
	if color {
		return ansiDim + block + ansiReset
	}
	return block
}

// trimPayload shortens s to at most max runes, appending an ellipsis when cut, so
// long JSON request bodies stay readable on one line.
func trimPayload(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
