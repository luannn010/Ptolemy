package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
)

// SynthScenario is a multi-session consolidation test: a build-up of atoms across
// sessions (some correcting earlier ones), then a procedural query whose consolidated
// answer should reflect current truth. ExpectKeywords must ALL appear in the recalled
// summary; RejectKeywords must NOT (they are superseded/stale claims).
//
// NOTE: this seed set is intentionally ~12 scenarios — below SPEC-GC §6b's 20–40
// target — and is designed to grow. Append scenarios over time.
type SynthScenario struct {
	ID             string   `json:"id"`
	Subject        string   `json:"subject"`
	Project        string   `json:"project"`
	Sessions       [][]Turn `json:"sessions"`
	Query          string   `json:"query"`
	ExpectKeywords []string `json:"expect_keywords"`
	RejectKeywords []string `json:"reject_keywords"`
}

type Turn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// ScoreSummary returns (allExpectedPresent, noRejectedPresent) for a recalled summary,
// case-insensitive. Pure — unit-tested without a DB or LLM.
func ScoreSummary(summary string, expect, reject []string) (bool, bool) {
	lower := strings.ToLower(summary)
	allExpected := true
	for _, e := range expect {
		if !strings.Contains(lower, strings.ToLower(e)) {
			allExpected = false
			break
		}
	}
	noRejected := true
	for _, r := range reject {
		if strings.Contains(lower, strings.ToLower(r)) {
			noRejected = false
			break
		}
	}
	return allExpected, noRejected
}

// LoadSynthScenarios reads the scenario JSON array.
func LoadSynthScenarios(path string) ([]SynthScenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenarios: %w", err)
	}
	var out []SynthScenario
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse scenarios: %w", err)
	}
	return out, nil
}

// RunSynthEval gates the 6b consolidation+recall pipeline. For each scenario it
// INJECTS the turns as atom rows directly (the assistant text holds the declarative
// fact), consolidates the (subject,project) via the live BRAIN LLM, recalls scoped to
// it, and scores the recalled synthesis summary. Atoms are injected rather than run
// through the per-turn extractor on purpose: extraction is 6a's concern (separately
// unit-tested + smoke-verified), and injecting keeps this gate deterministic and fast
// (one LLM call per scenario — the consolidation — instead of one per turn). Runs via
// `make eval-synth`, not `go test`. Returns pass count + failure descriptions.
func RunSynthEval(ctx context.Context, store memory.Store, embedder memory.Embedder, cons *memory.Consolidator, retr memory.Retriever, scenarios []SynthScenario) (passed int, failures []string, err error) {
	for _, sc := range scenarios {
		subj, proj := sc.Subject, sc.Project
		// Flatten turns in order; the assistant text is the atom content. Order matters:
		// later atoms (corrections) are created after earlier ones, so the consolidator's
		// created_at-ASC scan presents the correction last (newer truth).
		var contents []string
		for _, sess := range sc.Sessions {
			for _, turn := range sess {
				contents = append(contents, turn.Assistant)
			}
		}
		vecs, eerr := embedder.Embed(ctx, contents)
		if eerr != nil || len(vecs) != len(contents) {
			if eerr == nil {
				eerr = fmt.Errorf("embedder returned %d vectors for %d atoms", len(vecs), len(contents))
			}
			return passed, failures, fmt.Errorf("scenario %s embed: %w", sc.ID, eerr)
		}
		for i, content := range contents {
			s2, p2, pe2 := subj, proj, "factual"
			atom := memory.Chunk{
				ID:          fmt.Sprintf("%s-atom-%d", sc.ID, i),
				Content:     content,
				Embedding:   vecs[i],
				Scope:       "project",
				Status:      "active",
				Importance:  0.5,
				SubjectID:   &s2,
				ProjectID:   &p2,
				Perspective: &pe2,
				Metadata:    map[string]any{"kind": "atom"},
			}
			if uerr := store.Upsert(ctx, []memory.Chunk{atom}); uerr != nil {
				return passed, failures, fmt.Errorf("scenario %s upsert atom: %w", sc.ID, uerr)
			}
		}
		if cerr := cons.ConsolidateSubjectProject(ctx, subj, proj); cerr != nil {
			return passed, failures, fmt.Errorf("scenario %s consolidate: %w", sc.ID, cerr)
		}
		got, rerr := retr.Retrieve(ctx, memory.Query{Text: sc.Query, K: 8, SubjectID: &subj, ProjectID: &proj}, 32)
		if rerr != nil {
			return passed, failures, fmt.Errorf("scenario %s retrieve: %w", sc.ID, rerr)
		}
		summary := ""
		for _, rc := range got {
			if kind, _ := rc.Metadata["kind"].(string); kind == "synthesis" {
				summary = rc.Content
				break
			}
		}
		okExpect, noReject := ScoreSummary(summary, sc.ExpectKeywords, sc.RejectKeywords)
		if summary != "" && okExpect && noReject {
			passed++
		} else {
			failures = append(failures, fmt.Sprintf("%s: summary=%q expect=%v reject=%v (okExpect=%v noReject=%v)",
				sc.ID, summary, sc.ExpectKeywords, sc.RejectKeywords, okExpect, noReject))
		}
	}
	return passed, failures, nil
}
