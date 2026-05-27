// Package eval provides the pure logic for the memory module's retrieval
// eval harness: seed loading, hit detection by document-id prefix, and
// recall@k aggregation. The cmd/memory-eval CLI is a thin wrapper.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
)

// QuestionType tags a seed question so the harness can report per-type recall.
// Phase 3 introduces four buckets; only these literal strings are accepted.
type QuestionType string

const (
	QuestionParaphrase   QuestionType = "paraphrase"
	QuestionExactToken   QuestionType = "exact_token"
	QuestionFreshVsStale QuestionType = "fresh_vs_stale"
	QuestionNegative     QuestionType = "negative"
)

// fixtureVersion is stamped into Summary.FixtureVer and into the sweep table
// footer. Bump whenever the byte-stable fixtures under testdata/corpus/ are
// resynced to evolving real docs, so cross-PR sweep tables stay comparable.
const fixtureVersion = 1

var validQuestionTypes = map[QuestionType]bool{
	QuestionParaphrase:   true,
	QuestionExactToken:   true,
	QuestionFreshVsStale: true,
	QuestionNegative:     true,
}

// Seed mirrors the on-disk eval JSON.
type Seed struct {
	K         int          `json:"k"`
	Corpus    []CorpusItem `json:"corpus"`
	Questions []Question   `json:"questions"`
}

// CorpusItem points at a repo-local document the harness should ingest.
// ID becomes the doc id passed to Orchestrator.Ingest; the chunker then
// suffixes "#0", "#1", ... so a question's expected_doc_ids = []string{ID}
// matches any chunk derived from it.
type CorpusItem struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Question struct {
	ID             string       `json:"id"`
	Text           string       `json:"text"`
	ExpectedDocIDs []string     `json:"expected_doc_ids"`
	QuestionType   QuestionType `json:"question_type,omitempty"`
	Rationale      string       `json:"rationale,omitempty"`
}

type QuestionResult struct {
	Question  Question
	Retrieved []memory.RetrievedChunk
	Hits      []string // expected doc ids that the retrieved list covered
	Expected  []string // copy of question.ExpectedDocIDs for Summarize
}

type Summary struct {
	Total      int
	MeanRecall float64
	PerType    map[QuestionType]float64
	NPerType   map[QuestionType]int
	FixtureVer int
}

func LoadSeed(path string) (Seed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Seed{}, fmt.Errorf("read seed: %w", err)
	}
	var s Seed
	if err := json.Unmarshal(data, &s); err != nil {
		return Seed{}, fmt.Errorf("parse seed: %w", err)
	}
	for _, q := range s.Questions {
		if q.QuestionType == "" {
			// Backwards-compat: untagged questions are allowed (the old 8-question
			// seed had no tags). They are reported under the empty bucket by
			// Summarize. The Phase 3 seed populates the tag for every question.
			continue
		}
		if !validQuestionTypes[q.QuestionType] {
			return Seed{}, fmt.Errorf("seed question %q: unknown question_type %q", q.ID, q.QuestionType)
		}
	}
	if s.K <= 0 {
		s.K = 5
	}
	return s, nil
}

// HitsExpected returns the subset of expectedDocIDs that have at least one
// retrieved chunk whose id starts with "<expected>#". The "#" guard prevents
// "doc1" from matching "doc10".
func HitsExpected(retrieved []memory.RetrievedChunk, expectedDocIDs []string) []string {
	var hits []string
	for _, want := range expectedDocIDs {
		for _, rc := range retrieved {
			if strings.HasPrefix(rc.ID, want+"#") {
				hits = append(hits, want)
				break
			}
		}
	}
	return hits
}

// RunRetrieval executes the retriever for each question and returns per-question
// results. It deliberately does NOT call the Generator — recall@k is purely a
// retrieval-quality measure and LLM calls would slow the harness 10–100x.
func RunRetrieval(ctx context.Context, r memory.Retriever, s Seed) ([]QuestionResult, error) {
	depth := s.K * 4 // generous candidate depth; reranker fodder for Phase 3
	if depth < 20 {
		depth = 20
	}
	results := make([]QuestionResult, 0, len(s.Questions))
	for _, q := range s.Questions {
		got, err := r.Retrieve(ctx, memory.Query{Text: q.Text, K: s.K}, depth)
		if err != nil {
			return nil, fmt.Errorf("retrieve %s: %w", q.ID, err)
		}
		topK := got
		if len(topK) > s.K {
			topK = topK[:s.K]
		}
		results = append(results, QuestionResult{
			Question:  q,
			Retrieved: topK,
			Hits:      HitsExpected(topK, q.ExpectedDocIDs),
			Expected:  q.ExpectedDocIDs,
		})
	}
	return results, nil
}

// Summarize computes mean recall@k overall and per QuestionType. Questions
// with empty Expected are excluded from BOTH numerator and denominator — a
// malformed seed entry shouldn't silently drag the mean down. Untagged
// questions contribute to MeanRecall but NOT to any PerType bucket.
func Summarize(results []QuestionResult) Summary {
	out := Summary{
		Total:      len(results),
		PerType:    map[QuestionType]float64{},
		NPerType:   map[QuestionType]int{},
		FixtureVer: fixtureVersion,
	}
	if len(results) == 0 {
		return out
	}
	var sum float64
	var counted int
	perTypeSum := map[QuestionType]float64{}
	for _, r := range results {
		if len(r.Expected) == 0 {
			continue
		}
		recall := float64(len(r.Hits)) / float64(len(r.Expected))
		sum += recall
		counted++
		qt := r.Question.QuestionType
		if qt != "" {
			perTypeSum[qt] += recall
			out.NPerType[qt]++
		}
	}
	if counted > 0 {
		out.MeanRecall = sum / float64(counted)
	}
	for qt, s := range perTypeSum {
		if n := out.NPerType[qt]; n > 0 {
			out.PerType[qt] = s / float64(n)
		}
	}
	return out
}
