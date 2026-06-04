package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractorVersion is stamped into every captured row's metadata so entries are
// auditable and selectively re-extractable when the prompt changes.
const ExtractorVersion = "extract_v2"

//go:embed prompts/extract_v2.txt
var extractPromptTemplate string

//go:embed grammar/atom.gbnf
var atomGrammar string

// ChatClient is the minimal LLM surface the Extractor (and 6b Consolidator) need.
// OpenAIGenerator.Complete satisfies it; tests use a fake.
type ChatClient interface {
	Complete(ctx context.Context, system, user string, opts CompleteOptions) (string, error)
}

type extractorResponse struct {
	Atoms []Atom `json:"atoms"`
}

// CompleteOptions are optional generation controls supported by the backing model server.
type CompleteOptions struct {
	Grammar string
}

// Extractor turns an exchange into grounded, self-contained entries. The grounding
// and dangling-pronoun checks are deterministic and unit-tested with a fake client.
type Extractor struct {
	Client  ChatClient
	Grammar string
}

func NewExtractor(c ChatClient) *Extractor {
	return &Extractor{Client: c, Grammar: strings.TrimSpace(atomGrammar)}
}

func (e *Extractor) Extract(ctx context.Context, ex Exchange) ([]Atom, error) {
	// Instructions are the system prompt; the turn is the user message. Sending
	// the exchange once (not embedded in the system prompt AND repeated as user)
	// keeps this cheap on the per-turn capture path.
	turn := "USER:\n" + ex.UserText + "\n\nASSISTANT:\n" + ex.AssistantText
	raw, err := e.Client.Complete(ctx, extractPromptTemplate, turn, CompleteOptions{Grammar: e.Grammar})
	if err != nil {
		return nil, fmt.Errorf("extractor llm: %w", err)
	}
	parsed, err := parseEntries(raw)
	if err != nil {
		return nil, err
	}
	var out []Atom
	for _, en := range parsed {
		if strings.TrimSpace(en.Content) == "" {
			continue
		}
		if en.Perspective != "factual" && en.Perspective != "relational" {
			en.Perspective = "relational"
		}
		out = append(out, en)
	}
	return out, nil
}

func parseEntries(raw string) ([]Atom, error) {
	s := strings.TrimSpace(raw)
	var payload extractorResponse
	if err := json.Unmarshal([]byte(s), &payload); err != nil {
		return nil, fmt.Errorf("extractor parse: %w (raw=%q)", err, raw)
	}
	return payload.Atoms, nil
}
