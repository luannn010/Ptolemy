package eval

import (
	"context"
	"fmt"

	"github.com/luannn010/ptolemy/internal/memory"
)

// AgentResult is one question run through the agent loop (not just retrieval).
type AgentResult struct {
	Question  Question
	GaveUp    bool
	Citations []string
}

// AgentSummary aggregates give-up/grounding correctness over a seed.
type AgentSummary struct {
	Total         int
	GiveUpCorrect int // questions where give_up vs answer matched expectation
	Grounded      int // non-give_up answers carrying >=1 citation
	Answered      int // non-give_up count (denominator for grounding)
}

// answerer is the loop surface the agent eval needs. *memory.Orchestrator
// satisfies it (Answer delegates to the loop when AGENT_LOOP_ENABLED is on).
type answerer interface {
	Answer(ctx context.Context, q memory.Query) (memory.Answer, error)
}

// scoreGiveUpCorrect: a question is "answerable" iff it has expected doc ids.
// Answerable questions are correct iff the loop answered (did not give up);
// negative questions (no expected docs) are correct iff the loop gave up.
func scoreGiveUpCorrect(r AgentResult) bool {
	// "answerable" is proxied by having >=1 expected doc id. This holds only if
	// every non-negative seed question carries expected doc ids (negative
	// questions intentionally have none). LoadSeed does not enforce this, so keep
	// the invariant when adding seed questions or this scoring will silently skew.
	answerable := len(r.Question.ExpectedDocIDs) > 0
	if answerable {
		return !r.GaveUp
	}
	return r.GaveUp
}

// RunAgentEval runs every seed question through the agent loop and records the
// give_up flag + citations. Unlike RunRetrieval this DOES call the LLM (one or
// more BRAIN calls per question), so it is slow.
func RunAgentEval(ctx context.Context, a answerer, s Seed) ([]AgentResult, error) {
	results := make([]AgentResult, 0, len(s.Questions))
	for _, q := range s.Questions {
		ans, err := a.Answer(ctx, memory.Query{Text: q.Text, K: s.K})
		if err != nil {
			return nil, fmt.Errorf("agent answer %s: %w", q.ID, err)
		}
		results = append(results, AgentResult{Question: q, GaveUp: ans.GaveUp, Citations: ans.Citations})
	}
	return results, nil
}

// SummarizeAgent computes give-up correctness and grounding rates.
func SummarizeAgent(results []AgentResult) AgentSummary {
	out := AgentSummary{Total: len(results)}
	for _, r := range results {
		if scoreGiveUpCorrect(r) {
			out.GiveUpCorrect++
		}
		if !r.GaveUp {
			out.Answered++
			if len(r.Citations) > 0 {
				out.Grounded++
			}
		}
	}
	return out
}
