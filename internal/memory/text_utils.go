package memory

import (
	"regexp"
	"strings"
)

var jsonFence = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "and": true,
	"or": true, "in": true, "on": true, "for": true, "is": true, "are": true,
	"was": true, "be": true, "by": true, "as": true, "at": true, "it": true,
	"this": true, "that": true, "with": true, "from": true,
}

func normalizeTokens(s string) []string {
	parts := nonWord.Split(strings.ToLower(s), -1)
	var out []string
	for _, p := range parts {
		if p == "" || stopWords[p] {
			continue
		}
		out = append(out, stem(p))
	}
	return out
}

func stem(w string) string {
	for _, suf := range []string{"ization", "ation", "ing", "edly", "ed", "ly", "es", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}
