package memory

import (
	"regexp"
	"strings"
)

// GateDecision is the deterministic, LLM-free verdict for whether a turn is
// worth capturing. ContainsSecret records that the turn matched an obvious-secret
// pattern (6a skips such turns; 6b may redact instead).
type GateDecision struct {
	Skip           bool
	Reason         string
	ContainsSecret bool
}

// minCaptureRunes: turns shorter than this carry no durable signal.
const minCaptureRunes = 16

var trivialPhrases = map[string]bool{
	"hi": true, "hey": true, "hello": true, "yo": true,
	"thanks": true, "thank you": true, "thx": true, "ty": true,
	"ok": true, "okay": true, "k": true, "cool": true, "nice": true,
	"yes": true, "no": true, "yep": true, "nope": true, "sure": true,
	"got it": true, "great": true, "perfect": true, "sounds good": true,
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),                                              // OpenAI-style
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                                 // AWS access key id
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{20,}`),                               // bearer token
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),                               // PEM private key
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*\S{12,}`),         // labelled secret
}

// highEntropyToken flags a run that looks like a credential: long and drawing
// from at least three character classes (lower, upper, digit). No \b anchors —
// base64 padding (=,+,/) is non-word, so \b would miss a padded blob's edge.
var highEntropyToken = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{40,}`)

func containsSecret(s string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	for _, tok := range highEntropyToken.FindAllString(s, -1) {
		if charClasses(tok) >= 3 {
			return true
		}
	}
	return false
}

func charClasses(s string) int {
	var lower, upper, digit bool
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit} {
		if b {
			n++
		}
	}
	return n
}

// Gate is a pure, fully testable skip for turns not worth capturing.
func Gate(ex Exchange) GateDecision {
	combined := strings.TrimSpace(ex.UserText + " " + ex.AssistantText)
	if containsSecret(ex.UserText) || containsSecret(ex.AssistantText) {
		return GateDecision{Skip: true, Reason: "secret", ContainsSecret: true}
	}
	if len([]rune(combined)) < minCaptureRunes {
		return GateDecision{Skip: true, Reason: "too_short"}
	}
	if trivialPhrases[strings.ToLower(strings.TrimSpace(ex.UserText))] &&
		len([]rune(strings.TrimSpace(ex.AssistantText))) < minCaptureRunes {
		return GateDecision{Skip: true, Reason: "greeting"}
	}
	return GateDecision{Skip: false}
}
