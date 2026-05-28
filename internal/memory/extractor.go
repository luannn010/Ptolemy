package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ExtractorVersion is stamped into every captured row's metadata so entries are
// auditable and selectively re-extractable when the prompt changes.
const ExtractorVersion = "extract_v1"

//go:embed prompts/extract_v1.txt
var extractPromptTemplate string

// ChatClient is the minimal LLM surface the Extractor (and 6b Consolidator) need.
// OpenAIGenerator.Complete satisfies it; tests use a fake.
type ChatClient interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// ExtractedEntry is one atomic, self-contained memory candidate from a turn.
type ExtractedEntry struct {
	Content       string `json:"content"`
	Perspective   string `json:"perspective"`
	FactSubject   string `json:"fact_subject"`
	FactPredicate string `json:"fact_predicate"`
}

// Extractor turns an exchange into grounded, self-contained entries. The grounding
// and dangling-pronoun checks are deterministic and unit-tested with a fake client.
type Extractor struct {
	Client           ChatClient
	MinGroundOverlap float64 // fraction of entry content tokens that must appear in the exchange window
}

func NewExtractor(c ChatClient) *Extractor {
	return &Extractor{Client: c, MinGroundOverlap: 0.4}
}

func (e *Extractor) Extract(ctx context.Context, ex Exchange) ([]ExtractedEntry, error) {
	// Instructions are the system prompt; the turn is the user message. Sending
	// the exchange once (not embedded in the system prompt AND repeated as user)
	// keeps this cheap on the per-turn capture path.
	turn := "USER:\n" + ex.UserText + "\n\nASSISTANT:\n" + ex.AssistantText
	raw, err := e.Client.Complete(ctx, extractPromptTemplate, turn)
	if err != nil {
		return nil, fmt.Errorf("extractor llm: %w", err)
	}
	parsed, err := parseEntries(raw)
	if err != nil {
		return nil, err
	}
	window := normalizeTokens(ex.UserText + " " + ex.AssistantText)
	windowSet := tokenSet(window)
	var out []ExtractedEntry
	for _, en := range parsed {
		if strings.TrimSpace(en.Content) == "" {
			continue
		}
		if en.Perspective != "factual" && en.Perspective != "relational" {
			en.Perspective = "relational"
		}
		if hasDanglingPronoun(en.Content) {
			continue
		}
		if !isGrounded(en.Content, windowSet, e.MinGroundOverlap) {
			continue
		}
		out = append(out, en)
	}
	return out, nil
}

var jsonFence = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

func parseEntries(raw string) ([]ExtractedEntry, error) {
	s := strings.TrimSpace(raw)
	if m := jsonFence.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	if i := strings.Index(s, "["); i > 0 {
		s = s[i:]
	}
	var entries []ExtractedEntry
	if err := json.Unmarshal([]byte(s), &entries); err != nil {
		return nil, fmt.Errorf("extractor parse: %w (raw=%q)", err, raw)
	}
	return entries, nil
}

// clauseStartPronoun detects a dangling subject pronoun: one that opens the
// content or a sentence, with no antecedent available in isolation.
var clauseStartPronoun = regexp.MustCompile(`(?i)(^|[.;]\s+)(it|this|that|they|them|those|these|he|she)\b`)

func hasDanglingPronoun(content string) bool {
	return clauseStartPronoun.MatchString(strings.TrimSpace(content))
}

// isGrounded checks stemmed-token overlap of the entry against the exchange window.
func isGrounded(content string, windowSet map[string]bool, minOverlap float64) bool {
	toks := normalizeTokens(content)
	if len(toks) == 0 {
		return false
	}
	hit := 0
	for _, t := range toks {
		if windowSet[t] {
			hit++
		}
	}
	return float64(hit)/float64(len(toks)) >= minOverlap
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "and": true,
	"or": true, "in": true, "on": true, "for": true, "is": true, "are": true,
	"was": true, "be": true, "by": true, "as": true, "at": true, "it": true,
	"this": true, "that": true, "with": true, "from": true,
}

// normalizeTokens lowercases, splits on non-word chars, drops stopwords, and
// applies a light stemmer so "implemented"/"implementation"/"implements" match.
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

func tokenSet(toks []string) map[string]bool {
	m := make(map[string]bool, len(toks))
	for _, t := range toks {
		m[t] = true
	}
	return m
}

// stem strips common English suffixes. Deliberately crude (no Porter dep):
// enough to bridge implement/implemented/implementation and archive/archiving.
func stem(w string) string {
	for _, suf := range []string{"ization", "ation", "ing", "edly", "ed", "ly", "es", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}
