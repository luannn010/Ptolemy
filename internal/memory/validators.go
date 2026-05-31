package memory

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// Atom is one extracted, structured memory candidate.
type Atom struct {
	Content       string `json:"content"`
	Perspective   string `json:"perspective"`
	FactSubject   string `json:"fact_subject"`
	FactPredicate string `json:"fact_predicate"`
}

// TurnContext is the source exchange used for grounding and diagnostics.
type TurnContext struct {
	UserText      string
	AssistantText string
	SessionID     string
	SubjectID     string
	ProjectID     string
}

// AtomValidator validates one atom against deterministic rules.
type AtomValidator interface {
	Name() string
	Validate(atom Atom, source TurnContext) error
}

type NonEmptyValidator struct{}

func (NonEmptyValidator) Name() string { return "non_empty" }
func (NonEmptyValidator) Validate(atom Atom, _ TurnContext) error {
	if strings.TrimSpace(atom.Content) == "" {
		return fmt.Errorf("content is empty")
	}
	if strings.TrimSpace(atom.Perspective) == "" {
		return fmt.Errorf("perspective is empty")
	}
	return nil
}

type LengthBoundsValidator struct {
	MaxContent int
	MaxFact    int
}

func (LengthBoundsValidator) Name() string { return "length_bounds" }
func (v LengthBoundsValidator) Validate(atom Atom, _ TurnContext) error {
	maxContent := v.MaxContent
	if maxContent <= 0 {
		maxContent = 1000
	}
	maxFact := v.MaxFact
	if maxFact <= 0 {
		maxFact = 200
	}
	if len([]rune(atom.Content)) > maxContent {
		return fmt.Errorf("content length exceeds %d runes", maxContent)
	}
	if len([]rune(atom.FactSubject)) > maxFact {
		return fmt.Errorf("fact_subject length exceeds %d runes", maxFact)
	}
	if len([]rune(atom.FactPredicate)) > maxFact {
		return fmt.Errorf("fact_predicate length exceeds %d runes", maxFact)
	}
	return nil
}

var allowedPredicates = map[string]bool{
	"":             true,
	"uses":         true,
	"requires":     true,
	"runs_on":      true,
	"stores_in":    true,
	"listens_on":   true,
	"decides":      true,
	"prefers":      true,
	"archives":     true,
	"implements":   true,
	"configured_as": true,
}

type PredicateTaxonomyValidator struct{}

func (PredicateTaxonomyValidator) Name() string { return "predicate_taxonomy" }
func (PredicateTaxonomyValidator) Validate(atom Atom, _ TurnContext) error {
	p := strings.TrimSpace(atom.FactPredicate)
	if !allowedPredicates[p] {
		return fmt.Errorf("predicate %q not in allowed taxonomy", p)
	}
	return nil
}

var wsRe = regexp.MustCompile(`\s+`)

func normalizeForContains(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return wsRe.ReplaceAllString(s, " ")
}

type EvidenceInSourceValidator struct{}

func (EvidenceInSourceValidator) Name() string { return "evidence_in_source" }
func (EvidenceInSourceValidator) Validate(atom Atom, source TurnContext) error {
	needle := normalizeForContains(atom.Content)
	if needle == "" {
		return fmt.Errorf("empty normalized content")
	}
	hay := normalizeForContains(source.UserText + " " + source.AssistantText)
	if !strings.Contains(hay, needle) {
		return fmt.Errorf("content not found in source exchange")
	}
	return nil
}

type ValidatorChain struct {
	validators []AtomValidator
	onReject   func(validator, reason string, atom Atom, source TurnContext)
}

func NewDefaultValidatorChain() *ValidatorChain {
	return &ValidatorChain{
		validators: []AtomValidator{
			NonEmptyValidator{},
			LengthBoundsValidator{},
			PredicateTaxonomyValidator{},
			EvidenceInSourceValidator{},
		},
		onReject: defaultRejectLogger,
	}
}

func defaultRejectLogger(validator, reason string, atom Atom, source TurnContext) {
	log.Warn().
		Str("stage", "validate").
		Str("validator", validator).
		Str("reason", reason).
		Str("session_id", source.SessionID).
		Str("subject_id", source.SubjectID).
		Str("project_id", source.ProjectID).
		Str("content", atom.Content).
		Msg("capture: atom rejected")
}

func (c *ValidatorChain) Filter(atoms []Atom, source TurnContext) []Atom {
	if c == nil || len(c.validators) == 0 {
		return atoms
	}
	out := make([]Atom, 0, len(atoms))
	for _, atom := range atoms {
		rejected := false
		for _, v := range c.validators {
			if err := v.Validate(atom, source); err != nil {
				if c.onReject != nil {
					c.onReject(v.Name(), err.Error(), atom, source)
				}
				rejected = true
				break
			}
		}
		if !rejected {
			out = append(out, atom)
		}
	}
	return out
}
