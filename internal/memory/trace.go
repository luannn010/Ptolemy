package memory

import "strings"

// RecallTrace is the optional reasoning trace returned when Query.Trace is set.
// It is strictly additive: nil unless tracing was requested, and never affects
// the answer text or citations.
type RecallTrace struct {
	Mode  string      `json:"mode"` // "agentic" | "legacy"
	Steps []TraceStep `json:"steps"`
}

// TraceStep is one step of a recall: a planner action and its effects.
type TraceStep struct {
	Index       int          `json:"index"`
	Action      string       `json:"action"`                 // "retrieve" | "answer" | "give_up"
	Reason      string       `json:"reason,omitempty"`       // give_up reason
	Query       string       `json:"query,omitempty"`        // query a retrieve step issued
	Retrieved   []TraceChunk `json:"retrieved,omitempty"`    // per-step retrieved delta
	GaveUp      bool         `json:"gave_up,omitempty"`      // terminal give_up
	GroundingOK bool         `json:"grounding_ok,omitempty"` // terminal answer passed grounding
}

// TraceChunk is a compact view of one retrieved chunk.
type TraceChunk struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// snippet returns a single-line, rune-bounded preview of s. Newlines and
// carriage returns collapse to single spaces; if the result exceeds n runes it
// is cut to n runes and an ellipsis is appended.
func snippet(s string, n int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
