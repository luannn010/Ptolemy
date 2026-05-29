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

// RunSynthEval drives each scenario end-to-end: capture every turn synchronously,
// consolidate the (subject,project), recall scoped to it, and score the recalled
// synthesis summary. Uses the live BRAIN LLM (extractor + consolidator), so it runs
// via `make eval-synth`, not `go test`. Returns pass count + failure descriptions.
func RunSynthEval(ctx context.Context, hook *memory.PerTurnCaptureHook, cons *memory.Consolidator, retr memory.Retriever, scenarios []SynthScenario) (passed int, failures []string, err error) {
	for _, sc := range scenarios {
		for si, sess := range sc.Sessions {
			for _, turn := range sess {
				ex := memory.Exchange{
					UserText: turn.User, AssistantText: turn.Assistant,
					SubjectID: sc.Subject, SessionID: fmt.Sprintf("%s-s%d", sc.ID, si), ProjectID: sc.Project,
				}
				if cerr := hook.Capture(ctx, ex); cerr != nil {
					return passed, failures, fmt.Errorf("scenario %s capture: %w", sc.ID, cerr)
				}
			}
		}
		if cerr := cons.ConsolidateSubjectProject(ctx, sc.Subject, sc.Project); cerr != nil {
			return passed, failures, fmt.Errorf("scenario %s consolidate: %w", sc.ID, cerr)
		}
		subj, proj := sc.Subject, sc.Project
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
